package tts

import (
	"context"
	"fmt"
	"math"
	"time"

	"ebook-audiobook/internal/model"
)

// VoiceQualityReport contains voice quality evaluation metrics
type VoiceQualityReport struct {
	VoiceID    string  `json:"voice_id"`
	Engine     string  `json:"engine"`
	TestText   string  `json:"test_text"`
	AudioSize  int     `json:"audio_size_bytes"`
	DurationMs float64 `json:"duration_ms"`

	// MOS-like proxy metrics (range 0.0-5.0)
	SignalQuality float64 `json:"signal_quality"` // estimated signal-to-noise proxy
	SpeedScore    float64 `json:"speed_score"`    // speaking speed appropriateness
	OverallScore  float64 `json:"overall_score"`  // combined quality score

	Warnings []string `json:"warnings,omitempty"`
}

// VoiceEvaluator evaluates voice synthesis quality
type VoiceEvaluator struct {
	engineReg *EngineRegistry
}

// NewVoiceEvaluator creates a voice quality evaluator
func NewVoiceEvaluator(reg *EngineRegistry) *VoiceEvaluator {
	return &VoiceEvaluator{engineReg: reg}
}

// Evaluate synthesizes a test phrase and computes quality metrics
func (ve *VoiceEvaluator) Evaluate(ctx context.Context, vp *model.VoiceProfile) (*VoiceQualityReport, error) {
	testText := "你好，这是一段用于评估音色质量的测试文本。语音合成技术正在快速发展。"
	report := &VoiceQualityReport{
		VoiceID:  vp.ID,
		Engine:   vp.Engine,
		TestText: testText,
	}

	startTime := time.Now()
	audio, _, err := ve.engineReg.SynthesizeWithEngine(ctx, testText, vp, model.TTSOptions{
		VoiceID: vp.VoiceID,
		Format:  "wav",
	})
	if err != nil {
		return nil, fmt.Errorf("synthesize for evaluation: %w", err)
	}
	elapsed := time.Since(startTime)

	report.AudioSize = len(audio)
	report.DurationMs = float64(elapsed.Milliseconds())

	// Estimate audio duration from WAV header (byte 40-43 = data size, at 24000Hz 16-bit mono)
	estDurationSec := float64(len(audio)-44) / (24000.0 * 2.0) // subtract WAV header, mono 16-bit
	if estDurationSec < 0.1 {
		estDurationSec = 0.5 // minimum
	}

	// Compute quality metrics
	report.SignalQuality = estimateSignalQuality(audio)
	report.SpeedScore = estimateSpeedScore(testText, estDurationSec)
	report.OverallScore = math.Round((report.SignalQuality*0.5+report.SpeedScore*0.5)*100) / 100

	// Check for warnings
	if report.SignalQuality < 3.0 {
		report.Warnings = append(report.Warnings, "音频信噪比较低，可能含有背景噪声或失真")
	}
	if report.SpeedScore < 2.5 {
		report.Warnings = append(report.Warnings, "合成语速异常，请检查语音配置")
	}
	if report.AudioSize < 100 {
		report.Warnings = append(report.Warnings, "合成音频过短，可能失败")
	}

	return report, nil
}

// estimateSignalQuality computes a rough signal quality proxy from PCM data
// Range: 0.0 (noisy/silent) to 5.0 (clean)
func estimateSignalQuality(audio []byte) float64 {
	if len(audio) < 100 {
		return 0.0
	}

	// Skip WAV header (44 bytes), analyze PCM samples
	pcmData := audio[44:]
	if len(pcmData) < 100 {
		return 2.0 // default for very short audio
	}

	// Calculate RMS and peak
	var sumSq float64
	var peak float64
	samples := len(pcmData) / 2
	if samples < 10 {
		return 2.0
	}

	for i := 0; i < len(pcmData)-1; i += 2 {
		// 16-bit little-endian
		sample := float64(int16(pcmData[i]) | int16(pcmData[i+1])<<8)
		absVal := math.Abs(sample)
		if absVal > peak {
			peak = absVal
		}
		sumSq += sample * sample
	}

	rms := math.Sqrt(sumSq / float64(samples))

	// Signal quality proxy:
	// - Very low RMS → likely silence (bad)
	// - Good RMS with reasonable peak → clean signal
	// - Extreme peak with low RMS → clipping/noise
	rmsRatio := math.Min(rms/500.0, 1.0) // normalize to ~0-1
	crestFactor := peak / math.Max(rms, 1.0)

	if crestFactor > 50 {
		rmsRatio *= 0.5 // heavily compressed/clipped
	}
	if rms < 10 {
		rmsRatio = 0.1 // nearly silent
	}

	return math.Round(math.Min(rmsRatio*5.0, 5.0)*100) / 100
}

// estimateSpeedScore computes speaking speed appropriateness
// Expected: ~3-5 Chinese chars/second for natural speech
func estimateSpeedScore(text string, durationSec float64) float64 {
	charCount := float64(len([]rune(text)))
	if durationSec < 0.1 {
		durationSec = 0.5
	}
	charsPerSec := charCount / durationSec

	// Ideal range: 3-5 chars/sec
	ideal := 4.0
	deviation := math.Abs(charsPerSec-ideal) / ideal

	// Score: 5.0 = perfect, decreases with deviation
	score := 5.0 - deviation*5.0
	if score < 1.0 {
		score = 1.0
	}
	if score > 5.0 {
		score = 5.0
	}

	return math.Round(score*100) / 100
}
