// Copyright (c) 2017, RetailNext, Inc.

package gonut

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"os/exec"
	"testing"
	"time"
)

func TestUVarint(t *testing.T) {
	cases := []struct {
		input  []byte
		expect uint64
	}{
		{
			// the second byte should not be read
			input:  []byte{0x01, 0x02},
			expect: 0x01,
		},
		{
			input:  []byte{0x81, 0x00, 0x02},
			expect: 0x80, // 0x01 << 7
		},
		{
			input:  []byte{0x81, 0x02, 0x02},
			expect: 0x82, // 0x01 << 7 + 0x02
		},
	}

	for i, c := range cases {
		r := bytes.NewReader(c.input)
		got, err := readUvarint(r)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.expect {
			t.Errorf("%d: got %d != expect %d", i, got, c.expect)
		}
	}
}

func TestVarint(t *testing.T) {
	cases := []struct {
		input  []byte
		expect int64
	}{
		{
			// the second byte should not be read
			input:  []byte{0x01, 0x02},
			expect: 0x01,
		},
		{
			// uint == 0x80
			//   +1 == 0x81
			//  >>1 == 0x40
			//-0x40 == -64
			input:  []byte{0x81, 0x00, 0x02},
			expect: -64,
		},
		{
			input:  []byte{0x81, 0x01, 0x02},
			expect: 65, // (0x01 << 7 + 0x02)
		},
	}

	for i, c := range cases {
		r := bytes.NewReader(c.input)
		got, err := readVarint(r)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.expect {
			t.Errorf("%d: got %d != expect %d", i, got, c.expect)
		}
	}
}

func TestStream(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg binary not found")
		return
	}

	formatCmd := exec.Command(ffmpegPath, "-formats")
	formats, err := formatCmd.Output()
	if err != nil {
		t.Fatalf("Failed to read ffmpeg formats: %s", err)
	}

	if idx := bytes.Index(formats, []byte("image2pipe")); idx < 0 {
		t.Skipf("ffmpeg binary doesn't support image2pipe")
	}

	if idx := bytes.Index(formats, []byte("nut")); idx < 0 {
		t.Skipf("ffmpeg binary doesn't support nut")
	}

	blue := color.RGBA{0, 0, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	// fill blue
	img0 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img0, img0.Bounds(), &image.Uniform{blue}, image.ZP, draw.Src)

	// fill read
	img1 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img1, img1.Bounds(), &image.Uniform{red}, image.ZP, draw.Src)

	var png0 bytes.Buffer
	if err := png.Encode(&png0, img0); err != nil {
		t.Fatal(err)
	}

	var png1 bytes.Buffer
	if err := png.Encode(&png1, img1); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y", "-f", "image2pipe", "-vcodec", "png", "-r", "10", "-i", "pipe:0", "-f", "nut",
		"-vcodec", "rawvideo", "-pix_fmt", "rgb24", "-y", "pipe:")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	demuxer := NewDemuxer(stdout)
	if err != nil {
		t.Fatal(err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Start()

	go func() {
		stdin.Write(png0.Bytes())
		stdin.Write(png1.Bytes())
		stdin.Close()
	}()

	event, err := demuxer.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}

	if event.Type() != StartStreamEvent {
		t.Fatalf("Expected StartStreamEvent but got %v", event.Type())
	}

	ss := event.(StartVideoStream)
	if ss.Width() != 100 {
		t.Fatalf("Expected width 100 but got %d", ss.Width())
	}
	if ss.Height() != 100 {
		t.Fatalf("Expected height 100 but got %d", ss.Height())
	}

	event, err = demuxer.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type() != FrameEvent {
		t.Fatalf("Expected frame event but got %v", event.Type())
	}
	f := event.(Frame)

	rawData, err := ioutil.ReadAll(f.Data())
	if err != nil {
		t.Fatal(err)
	}

	if len(rawData) != 100*100*3 {
		t.Fatalf("Len mismatch expected %d but got %d", len(rawData), 100*100*3)
	}
	expectBytes := []byte{0x00, 0x00, 0xff}
	for i := 0; i < 100*100; i++ {
		for j, expect := range expectBytes {
			got := rawData[i*3+j]
			if got != expect {
				t.Fatalf("pix=%d (rbg=%d) expected %d but was %d", i, j, expect, got)
			}
		}
	}

	event, err = demuxer.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type() != FrameEvent {
		t.Fatalf("Expected frame event but got %v", event.Type())
	}
	f = event.(Frame)

	rawData, err = ioutil.ReadAll(f.Data())
	if err != nil {
		t.Fatal(err)
	}

	if len(rawData) != 100*100*3 {
		t.Fatalf("Len mismatch expected %d but got %d", len(rawData), 100*100*3)
	}
	expectBytes = []byte{0xff, 0x00, 0x00}
	for i := 0; i < 100*100; i++ {
		for j, expect := range expectBytes {
			got := rawData[i*3+j]
			if got != expect {
				t.Fatalf("pix=%d (rbg=%d) expected %d but was %d", i, j, expect, got)
			}
		}
	}
}
