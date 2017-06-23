// Copyright (c) 2017, RetailNext, Inc.

package gonut

var (
	fileID = []byte("nut/multimedia container\x00")

	mainStartCode      = [8]byte{'N', 'M', 0x7a, 0x56, 0x1f, 0x5f, 0x04, 0xad}
	streamStartCode    = [8]byte{'N', 'S', 0x11, 0x40, 0x5B, 0xF2, 0xF9, 0xDB}
	syncpointStartCode = [8]byte{'N', 'K', 0xE4, 0xAD, 0xEE, 0xCA, 0x45, 0x69}
	indexStartCode     = [8]byte{'N', 'X', 0xDD, 0x67, 0x2F, 0x23, 0xE6, 0x4E}
	infoStartCode      = [8]byte{'N', 'I', 0xAB, 0x68, 0xB5, 0x96, 0xBA, 0x78}
)

type flag int

const (
	flagKey       flag = 1    // the frame is a keyframe.
	flagEOR            = 2    // the stream has no relevance on	presentation. (EOR)
	flagCodedPts       = 8    // coded_pts is in the frame header.
	flagStreamID       = 16   // stream_id is coded in the frame header.
	flagSizeMSB        = 32   // data_size_msb is coded in the frame header, otherwise data_size_msb is 0.
	flagChecksum       = 64   // the frame header contains a checksum.
	flagReserved       = 128  // reserved_count is coded in the frame header.
	flagHeaderIdx      = 1024 // header_idx is coded in the frame header.
	flagMatchTime      = 2048 // match_time_delta is coded in the frame header
	flagCoded          = 4096 // coded_flags are stored in the frame header.
	flagInvalid        = 8192 // frame_code is invalid.
)
