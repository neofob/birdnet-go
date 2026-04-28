package language

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloat32ToPCM16_Roundtrip(t *testing.T) {
	t.Parallel()

	samples := []float32{0.0, 0.5, -0.5, 1.0, -1.0, 0.123, -0.456}
	pcmData := Float32ToPCM16(samples)

	require.Len(t, pcmData, len(samples)*2)

	recovered := PCM16ToFloat32(pcmData)
	require.Len(t, recovered, len(samples))

	for i, original := range samples {
		assert.InDelta(t, original, recovered[i], 0.0001, "sample %d mismatch", i)
	}
}

func TestFloat32ToPCM16_Empty(t *testing.T) {
	t.Parallel()

	result := Float32ToPCM16([]float32{})
	assert.Empty(t, result)
}

func TestPCM16ToFloat32_OddBytes(t *testing.T) {
	t.Parallel()

	result := PCM16ToFloat32([]byte{0x01, 0x02, 0x03})
	assert.Nil(t, result)
}

func TestPCM16ToFloat32_Empty(t *testing.T) {
	t.Parallel()

	result := PCM16ToFloat32([]byte{})
	assert.Empty(t, result)
}

func TestFloat32ToWAV_Header(t *testing.T) {
	t.Parallel()

	samples := make([]float32, 48000)
	wavData := Float32ToWAV(samples, 48000)

	require.GreaterOrEqual(t, len(wavData), 44)
	assert.Equal(t, "RIFF", string(wavData[0:4]))
	assert.Equal(t, "WAVE", string(wavData[8:12]))
	assert.Equal(t, "fmt ", string(wavData[12:16]))
	assert.Equal(t, "data", string(wavData[36:40]))
}

func TestPCM16ToWAV(t *testing.T) {
	t.Parallel()

	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}
	wavData := PCM16ToWAV(pcmData, 16000)

	require.GreaterOrEqual(t, len(wavData), 44)
	assert.Equal(t, "RIFF", string(wavData[0:4]))
	assert.Equal(t, "WAVE", string(wavData[8:12]))
}

func TestResamplePCM16_SameRate(t *testing.T) {
	t.Parallel()

	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}
	result, err := ResamplePCM16(pcmData, 48000, 48000)

	require.NoError(t, err)
	assert.Equal(t, pcmData, result)
}

func TestResamplePCM16_Downsample(t *testing.T) {
	t.Parallel()

	sampleCount := 48000
	pcmData := make([]byte, sampleCount*2)
	result, err := ResamplePCM16(pcmData, 48000, 16000)

	require.NoError(t, err)
	expectedLen := 16000 * 2
	assert.InDelta(t, expectedLen, len(result), float64(expectedLen)*0.02,
		"resampled length should be approximately %d bytes", expectedLen)
}
