// Package compress provides zstd compression and decompression.
package compress

import (
	"github.com/klauspost/compress/zstd"
)

// encoder is a package-level reusable zstd encoder initialized at startup.
// It uses SpeedDefault (level 3) for a good balance of speed and compression ratio.
var encoder *zstd.Encoder

// decoder is a package-level reusable zstd decoder initialized at startup.
var decoder *zstd.Decoder

func init() {
	var err error
	encoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		panic("compress: failed to create zstd encoder: " + err.Error())
	}
	decoder, err = zstd.NewReader(nil)
	if err != nil {
		panic("compress: failed to create zstd decoder: " + err.Error())
	}
}

// Compress compresses data using zstd at level 3 (SpeedDefault).
func Compress(data []byte) ([]byte, error) {
	return encoder.EncodeAll(data, make([]byte, 0, len(data))), nil
}

// Decompress decompresses zstd-compressed data.
func Decompress(data []byte) ([]byte, error) {
	return decoder.DecodeAll(data, nil)
}
