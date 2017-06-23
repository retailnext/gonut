// Copyright (c) 2017, RetailNext, Inc.

package gonut

type sideName struct {
	name []byte
}

func (s sideName) Name() string {
	return string(s.name)
}

type sideUTF8 struct {
	sideName
	value string
}

type sideGeneric struct {
	sideName
	innerType []byte
	value     []byte
}

type sideInt64 struct {
	sideName
	value int64
}

type sideUint64 struct {
	sideName
	value uint64
}

type sideTime struct {
	sideName
	value uint64
}

type sideRational struct {
	sideName
	den int64
	num int64
}

type sideData interface {
	Name() string
}

func (p *rawPacket) readSideData() []sideData {
	if p.err != nil {
		return nil
	}

	count := p.readUvarint()
	out := make([]sideData, count)
	for i := uint64(0); i < count; i++ {
		name := p.readVarBytes()
		typeVal := p.readVarint()

		sideName := sideName{name}

		if typeVal == -1 {
			val := p.readVarBytes()
			out[i] = sideUTF8{
				sideName: sideName,
				value:    string(val),
			}
		} else if typeVal == -2 {
			innerType := p.readVarBytes()
			val := p.readVarBytes()
			out[i] = sideGeneric{
				sideName:  sideName,
				innerType: innerType,
				value:     val,
			}
		} else if typeVal == -3 {
			val := p.readVarint()
			out[i] = sideInt64{
				sideName: sideName,
				value:    val,
			}
		} else if typeVal == -4 {
			val := p.readUvarint()
			out[i] = sideTime{
				sideName: sideName,
				value:    val,
			}
		} else if typeVal < -4 {
			num := p.readVarint()
			out[i] = sideRational{
				sideName: sideName,
				den:      -typeVal - 4,
				num:      num,
			}
		} else {
			out[i] = sideUint64{
				sideName: sideName,
			}
		}
	}

	return out
}
