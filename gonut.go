// Copyright (c) 2017, RetailNext, Inc.

// gonut implements the FFMPEG NUT open container format.
// See https://ffmpeg.org/~michael/nut.txt for protocol details.
package gonut

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
)

type Demuxer struct {
	r              io.Reader
	mainHeader     *mainHeader
	err            error
	readHeaderOnce sync.Once
}

func NewDemuxer(r io.Reader) *Demuxer {
	return &Demuxer{
		r: r,
	}
}

type EventType int

const (
	StartStreamEvent EventType = iota
	FrameEvent
)

type Frame interface {
	Event
	StreamID() int
	Data() io.Reader
}

type StartStream interface {
	Event
	StreamID() int
	StreamClass() StreamClass
}

type StartVideoStream interface {
	StartStream
	// Width of video in pixels
	Width() int
	// Height of video in pixels
	Height() int
	// Horizontal distance between samples. Zero if unknown.
	SampleWidth() int
	// Veritical distance between samples. Zero if unknown.
	SampleHeight() int
}

type StartAudioStream interface {
	StartStream
	SampleRate() float64
	Channels() int
}

type Event interface {
	Type() EventType
}

type rawPacket struct {
	r   *bufio.Reader
	err error
}

type startStream struct {
	streamID int
}

func (d *Demuxer) ReadEvent() (Event, error) {
	d.readHeaderOnce.Do(func() {
		if err := d.readFileHeader(); err != nil {
			d.err = err
		}
	})

	for {
		if d.err != nil {
			return nil, d.err
		}

		var nextByte [1]byte
		_, err := io.ReadFull(d.r, nextByte[:])
		if err != nil {
			d.err = err
			return nil, d.err
		}

		if nextByte[0] == 'N' {
			header, err := d.readPacketHeader()
			if err != nil {
				d.err = err
				return nil, d.err
			}

			p := &rawPacket{
				r: bufio.NewReader(io.LimitReader(d.r, int64(header.packetSize))),
			}

			switch header.code {
			case mainStartCode:
				if d.mainHeader != nil {
					d.err = errors.New("Second Main header detected")
					return nil, d.err
				} else {
					d.mainHeader, err = p.readMainHeader()
				}
			case streamStartCode:
				header, err := p.readStreamHeader()
				if err != nil {
					d.err = err
					return nil, d.err
				}
				switch header.StreamClass() {
				case VideoClass:
					return &videoStream{*header}, nil
				case AudioClass:
					return &audioStream{*header}, nil
				default:
					return header, nil
				}
			case infoStartCode:
				_, err := p.readInfoPacket()
				if err != nil {
					d.err = err
					return nil, d.err
				}
			case syncpointStartCode:
				_, err := p.readSyncPoint()
				if err != nil {
					d.err = err
					return nil, d.err
				}
			case indexStartCode:
				_, err := d.readIndex(p)
				if err != nil {
					d.err = err
					return nil, d.err
				}
			default:
				d.err = fmt.Errorf("Unknown start code %v", header.code)
				return nil, d.err
			}
		} else {
			frame, err := d.readFrame(nextByte[0], d.mainHeader)
			if err != nil {
				d.err = err
				return nil, d.err
			}
			return frame, nil
		}

	}
}

func readUvarint(r io.Reader) (uint64, error) {
	var x uint64
	for i := 0; i < 9; i++ {
		var b [1]byte
		_, err := io.ReadFull(r, b[:])
		if err != nil {
			return x, err
		}
		x = (x << 7) | uint64(b[0]&0x7f)
		if b[0] < 0x80 {
			return x, nil
		}
	}

	return x, errors.New("varint overflows uint64")
}

func readVarint(r io.Reader) (int64, error) {
	u, err := readUvarint(r)
	if err != nil {
		return 0, err
	}
	u++
	var val int64
	if u&0x01 != 0 {
		val = -1 * int64(u>>1)
	} else {
		val = int64(u >> 1)
	}
	return val, nil
}

func (p *rawPacket) readUvarint() uint64 {
	if p.err != nil {
		return 0
	}
	uint, err := readUvarint(p.r)
	if err != nil {
		p.err = err
	}
	return uint
}

func (d *Demuxer) readUvarint() uint64 {
	if d.err != nil {
		return 0
	}
	uint, err := readUvarint(d.r)
	if err != nil {
		d.err = err
	}
	return uint
}

func (p *rawPacket) readVarBytes() []byte {
	if p.err != nil {
		return nil
	}
	byteCount, err := readUvarint(p.r)
	if err != nil {
		p.err = err
	}

	data := make([]byte, byteCount)
	if _, err := io.ReadFull(p.r, data); err != nil {
		p.err = err
	}

	return data
}

func (p *rawPacket) readVarint() int64 {
	if p.err != nil {
		return 0
	}
	i, err := readVarint(p.r)
	if err != nil {
		p.err = err
	}
	return i
}

func (d *Demuxer) readVarint() int64 {
	if d.err != nil {
		return 0
	}
	i, err := readVarint(d.r)
	if err != nil {
		d.err = err
	}
	return i
}

func (d *Demuxer) readFileHeader() error {
	fileIDBuf := make([]byte, len(fileID))
	_, err := io.ReadFull(d.r, fileIDBuf)
	if err != nil {
		return fmt.Errorf("Error reading file id: %s", err)
	}

	return nil
}

type PacketHeader struct {
	code       [8]byte
	packetSize uint64
	checksum   [4]byte
}

func (d *Demuxer) readPacketHeader() (PacketHeader, error) {
	var header PacketHeader

	header.code[0] = 'N'

	_, err := io.ReadFull(d.r, header.code[1:])
	if err != nil {
		d.err = err
		return header, err
	}

	header.packetSize, err = readUvarint(d.r)
	if err != nil {
		d.err = err
		return header, err
	}
	if header.packetSize > 4096 {
		_, err = io.ReadFull(d.r, header.checksum[:])
		if err != nil {
			d.err = err
			return header, err
		}
	}

	return header, d.err
}

type mainHeader struct {
	Version      uint64
	MinorVersion uint64
	StreamCount  uint64
	MaxDistance  uint64
	TimeBases    []Rational
	Frames       []frameInfo
	Flags        uint64
}

type frameInfo struct {
	flags          uint64
	streams        uint64
	mul            uint64
	lsb            uint64
	ptsDelta       int64
	reservedCount  uint64
	matchTimeDelta int64
	headerIdx      uint64
	streamID       uint64
}

func (p *rawPacket) readMainHeader() (*mainHeader, error) {
	var h mainHeader
	if p.err != nil {
		return nil, p.err
	}
	h.Version = p.readUvarint()
	if h.Version > 3 {
		h.MinorVersion = p.readUvarint()
	}
	h.StreamCount = p.readUvarint()
	h.MaxDistance = p.readUvarint()
	timeBaseCount := p.readUvarint()

	h.TimeBases = make([]Rational, timeBaseCount)
	for i := uint64(0); i < timeBaseCount; i++ {
		h.TimeBases[i] = Rational{
			numerator:   p.readUvarint(),
			denominator: p.readUvarint(),
		}
	}

	var (
		pts     int64
		mul     uint64 = 1
		stream  uint64 = 0
		match   int64  = 1 - (1 << 62)
		headIdx uint64 = 0
	)

	h.Frames = make([]frameInfo, 256)
	for i := 0; i < 256; {
		flags := p.readUvarint()
		fields := p.readUvarint()
		if fields > 0 {
			pts = p.readVarint()
		}
		if fields > 1 {
			mul = p.readUvarint()
		}
		if fields > 2 {
			stream = p.readUvarint()
		}

		var size uint64
		if fields > 3 {
			size = p.readUvarint()
		}

		var res uint64
		if fields > 4 {
			res = p.readUvarint()
		}

		count := mul - size
		if fields > 5 {
			count = p.readUvarint()
		}

		if fields > 6 {
			match = p.readVarint()
		}

		if fields > 7 {
			headIdx = p.readUvarint()
		}

		for j := uint64(8); j < fields; j++ {
			// seek past unknown fields
			p.readUvarint()
		}

		for j := uint64(0); j < count; j, i = j+1, i+1 {
			if i == 0x4E { //'N'
				h.Frames[i].flags = flagInvalid
				j--
				continue
			}
			h.Frames[i].flags = flags
			h.Frames[i].streamID = stream
			h.Frames[i].mul = mul
			h.Frames[i].lsb = size + j
			h.Frames[i].ptsDelta = pts
			h.Frames[i].reservedCount = res
			h.Frames[i].matchTimeDelta = match
			h.Frames[i].headerIdx = headIdx
		}
	}

	headerCount := p.readUvarint()
	headerCount++
	for i := uint64(1); i < headerCount; i++ {
		// seek past elision_header
		p.readVarBytes()
	}
	h.Flags = p.readUvarint()

	return &h, p.err
}

type videoStreamHeader struct {
	width          uint64
	height         uint64
	sampleWidth    uint64
	sampleHeight   uint64
	colorSpaceType uint64
}

type auditStreamHeader struct {
	sampleRateNum   uint64
	sampleRateDenom uint64
	channelCount    uint64
}

type Rational struct {
	numerator   uint64
	denominator uint64
}

func (r Rational) float64() float64 {
	if r.denominator == 0 {
		return 0
	}
	return float64(r.numerator) / float64(r.denominator)
}

type streamHeader struct {
	streamID          uint64
	streamClass       StreamClass
	fourcc            []byte
	timeBaseID        uint64
	msbPtsShift       uint64
	maxPtsDistance    uint64
	decodeDelay       uint64
	streamFlags       uint64
	codecSpecific     []byte
	videoStreamHeader *videoStreamHeader
	auditStreamHeader *auditStreamHeader
}

type videoStream struct {
	streamHeader
}

// Width of video in pixels
func (s *videoStream) Width() int {
	return int(s.videoStreamHeader.width)
}

// Height of video in pixels
func (s *videoStream) Height() int {
	return int(s.videoStreamHeader.height)
}

// Horizontal distance between samples. Zero if unknown.
func (s *videoStream) SampleWidth() int {
	return int(s.videoStreamHeader.sampleWidth)
}

// Veritical distance between samples. Zero if unknown.
func (s *videoStream) SampleHeight() int {
	return int(s.videoStreamHeader.sampleHeight)
}

type audioStream struct {
	streamHeader
}

func (s *audioStream) SampleRate() float64 {
	return float64(s.auditStreamHeader.sampleRateNum) / float64(s.auditStreamHeader.sampleRateDenom)
}

func (s *audioStream) Channels() int {
	return int(s.auditStreamHeader.channelCount)
}

func (s *streamHeader) StreamID() int {
	return int(s.streamID)
}

func (s *streamHeader) Type() EventType {
	return StartStreamEvent
}

func (s *streamHeader) StreamClass() StreamClass {
	return s.streamClass
}

type StreamClass byte

const (
	VideoClass     StreamClass = 0
	AudioClass                 = 1
	SubtitlesClass             = 2
	UserData                   = 3
)

func (p *rawPacket) readStreamHeader() (*streamHeader, error) {
	if p.err != nil {
		return nil, p.err
	}
	h := streamHeader{
		streamID:       p.readUvarint(),
		streamClass:    StreamClass(p.readUvarint()),
		fourcc:         p.readVarBytes(),
		timeBaseID:     p.readUvarint(),
		msbPtsShift:    p.readUvarint(),
		maxPtsDistance: p.readUvarint(),
		decodeDelay:    p.readUvarint(),
		streamFlags:    p.readUvarint(),
		codecSpecific:  p.readVarBytes(),
	}

	switch h.streamClass {
	case VideoClass:
		h.videoStreamHeader = &videoStreamHeader{
			width:          p.readUvarint(),
			height:         p.readUvarint(),
			sampleWidth:    p.readUvarint(),
			sampleHeight:   p.readUvarint(),
			colorSpaceType: p.readUvarint(),
		}
	case AudioClass:
		h.auditStreamHeader = &auditStreamHeader{
			sampleRateNum:   p.readUvarint(),
			sampleRateDenom: p.readUvarint(),
			channelCount:    p.readUvarint(),
		}
	}

	return &h, p.err
}

type pts float64

func (d *Demuxer) toTime(v uint64) pts {
	id := v % uint64(len(d.mainHeader.TimeBases))
	val := float64(v/uint64(len(d.mainHeader.TimeBases))) * d.mainHeader.TimeBases[id].float64()
	return pts(val)
}

type index struct {
	maxPTS            pts
	syncpointPOSDiv16 []uint64
}

func (d *Demuxer) readIndex(p *rawPacket) (*index, error) {
	// not implemented
	var i index
	return &i, nil
}

type infoPacket struct {
	streamID     uint64
	chapterID    int64
	chapterStart uint64 // time_base not accounted for
	chapterLen   uint64
	metaData     []sideData
}

func (p *rawPacket) readInfoPacket() (*infoPacket, error) {
	var i infoPacket
	if p.err != nil {
		return nil, p.err
	}

	i.streamID = p.readUvarint()
	i.chapterID = p.readVarint()
	i.chapterStart = p.readUvarint()
	i.chapterLen = p.readUvarint()

	i.metaData = p.readSideData()

	return &i, p.err
}

type syncPoint struct {
	globalKeyPts uint64
	backPtrDiv64 uint64
	// transmitTS   uint64
}

func (p *rawPacket) readSyncPoint() (*syncPoint, error) {
	var s syncPoint
	if p.err != nil {
		return nil, p.err
	}

	s.globalKeyPts = p.readUvarint()
	s.backPtrDiv64 = p.readUvarint()

	return &s, p.err
}

type frame struct {
	streamID       uint64
	codedPTS       uint64
	dataSizeMsb    uint64
	matchTimeDelta int64
	headerIdx      uint64
	res            uint64
	data           []byte
	dataAccessed   bool
}

func (d *Demuxer) readFrame(code byte, h *mainHeader) (*frame, error) {
	var f frame
	if d.err != nil {
		return nil, d.err
	}

	meta := h.Frames[code]

	f.streamID = meta.streamID
	f.matchTimeDelta = meta.matchTimeDelta
	f.headerIdx = meta.headerIdx

	size := meta.lsb
	sizeMul := meta.mul

	flags := meta.flags
	if flags&flagCoded > 0 {
		codedFlags := d.readUvarint()
		flags = flags ^ codedFlags
	}

	if flags&flagStreamID > 0 {
		f.streamID = d.readUvarint()
	}

	if flags&flagCodedPts > 0 {
		f.codedPTS = d.readUvarint()
	}

	if flags&flagSizeMSB > 0 {
		f.dataSizeMsb = d.readUvarint()
		size = size + sizeMul*f.dataSizeMsb
	}

	if flags&flagMatchTime > 0 {
		f.matchTimeDelta = d.readVarint()
	}

	if flags&flagHeaderIdx > 0 {
		f.headerIdx = d.readUvarint()
	}

	if flags&flagReserved > 0 {
		f.res = d.readUvarint()
	}

	for i := uint64(0); i < f.res; i++ {
		d.readUvarint()
	}

	if flags&flagChecksum > 0 {
		var sum [4]byte
		_, err := io.ReadFull(d.r, sum[:])
		if err != nil {
			d.err = err
			return nil, d.err
		}
	}

	f.data = make([]byte, size)
	_, err := io.ReadFull(d.r, f.data)
	if err != nil {
		d.err = err
		return nil, d.err
	}

	return &f, nil
}

func (f *frame) Type() EventType {
	return FrameEvent
}

func (f *frame) StreamID() int {
	return int(f.streamID)
}

func (f *frame) Data() io.Reader {
	if f.dataAccessed {
		// don't let you call Data() more than once for a frame
		// to make it easier to stop using a bytes buffer in the
		// future
		return nil
	}
	f.dataAccessed = true
	return bytes.NewReader(f.data)
}
