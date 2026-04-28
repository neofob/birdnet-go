package language

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tphakala/birdnet-go/internal/audiocore/resample"
	"github.com/tphakala/birdnet-go/internal/errors"
)

const (
	pcm16BytesPerSample = 2
	pcm16Scale          = 32768.0
	pcm16MaxPositive    = 32767.0
	wavHeaderSize       = 44
)

// Float32ToPCM16 converts float32 audio samples in [-1.0, 1.0] to 16-bit
// little-endian PCM bytes.
func Float32ToPCM16(samples []float32) []byte {
	buf := make([]byte, len(samples)*pcm16BytesPerSample)
	for i, s := range samples {
		clamped := math.Max(-1.0, math.Min(1.0, float64(s)))
		val := int16(clamped * pcm16MaxPositive)
		binary.LittleEndian.PutUint16(buf[i*pcm16BytesPerSample:], uint16(val)) //nolint:gosec // G115: intentional int16→uint16 bit reinterpretation
	}
	return buf
}

// PCM16ToFloat32 converts 16-bit little-endian PCM bytes to float32 samples
// in [-1.0, 1.0].
func PCM16ToFloat32(pcmData []byte) []float32 {
	if len(pcmData)%pcm16BytesPerSample != 0 {
		return nil
	}
	n := len(pcmData) / pcm16BytesPerSample
	samples := make([]float32, n)
	for i := range n {
		val := int16(binary.LittleEndian.Uint16(pcmData[i*pcm16BytesPerSample:])) //nolint:gosec // G115: intentional uint16→int16 bit reinterpretation
		samples[i] = float32(val) / float32(pcm16Scale)
	}
	return samples
}

// ResamplePCM16 resamples 16-bit PCM audio from fromRate to toRate.
// Returns the original data unchanged if fromRate == toRate.
func ResamplePCM16(pcmData []byte, fromRate, toRate int) ([]byte, error) {
	r, err := resample.NewResampler(fromRate, toRate)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return pcmData, nil
	}
	defer r.Close()

	out, err := r.ResampleInto(pcmData)
	if err != nil {
		return nil, err
	}

	result := make([]byte, len(out))
	copy(result, out)
	return result, nil
}

// Float32ToWAV encodes float32 samples as a 16-bit PCM WAV file.
func Float32ToWAV(samples []float32, sampleRate int) []byte {
	pcmData := Float32ToPCM16(samples)
	return encodeWAV(pcmData, sampleRate, len(samples))
}

// PCM16ToWAV wraps raw 16-bit PCM bytes in a WAV header.
func PCM16ToWAV(pcmData []byte, sampleRate int) []byte {
	numSamples := len(pcmData) / pcm16BytesPerSample
	return encodeWAV(pcmData, sampleRate, numSamples)
}

// PCM16ToWAVResampled resamples 16-bit PCM from fromRate to toRate and wraps
// it in a WAV header suitable for Whisper (16kHz mono S16 LE).
func PCM16ToWAVResampled(pcmData []byte, fromRate, toRate int) ([]byte, error) {
	resampled, err := ResamplePCM16(pcmData, fromRate, toRate)
	if err != nil {
		return nil, errors.Newf("failed to resample audio for WAV encoding: %w", err).
			Component("classifier.language").
			Category(errors.CategoryAudio).
			Context("from_rate", fromRate).
			Context("to_rate", toRate).
			Build()
	}
	numSamples := len(resampled) / pcm16BytesPerSample
	return encodeWAV(resampled, toRate, numSamples), nil
}

func encodeWAV(pcmData []byte, sampleRate, numSamples int) []byte {
	dataSize := len(pcmData)
	fileSize := wavHeaderSize + dataSize

	buf := new(bytes.Buffer)
	buf.Grow(fileSize)

	buf.WriteString("RIFF")
	binary.Write(buf, binary.BigEndian, uint32(fileSize-8)) //nolint:errcheck // bytes.Buffer never fails
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(buf, binary.BigEndian, uint32(16)) //nolint:errcheck // chunk size
	binary.Write(buf, binary.BigEndian, uint16(1))  //nolint:errcheck // PCM format
	binary.Write(buf, binary.BigEndian, uint16(1))  //nolint:errcheck // mono
	binary.Write(buf, binary.BigEndian, uint32(sampleRate)) //nolint:errcheck
	byteRate := sampleRate * pcm16BytesPerSample
	binary.Write(buf, binary.BigEndian, uint32(byteRate))  //nolint:errcheck
	binary.Write(buf, binary.BigEndian, uint16(pcm16BytesPerSample)) //nolint:errcheck // block align
	binary.Write(buf, binary.BigEndian, uint16(16))        //nolint:errcheck // bits per sample

	buf.WriteString("data")
	binary.Write(buf, binary.BigEndian, uint32(dataSize)) //nolint:errcheck
	buf.Write(pcmData)

	if buf.Len() != fileSize {
		panic(fmt.Sprintf("WAV encoding size mismatch: got %d, want %d", buf.Len(), fileSize))
	}

	return buf.Bytes()
}
