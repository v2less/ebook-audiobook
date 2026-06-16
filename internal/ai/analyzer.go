package ai

import (
	"context"
	"fmt"
	"strings"

	"ebook-audiobook/internal/llm"
	"ebook-audiobook/internal/model"
)

// Analyzer uses LLM to deeply understand novel text
type Analyzer struct {
	client *llm.Client
}

func NewAnalyzer(client *llm.Client) *Analyzer {
	return &Analyzer{client: client}
}

// ScriptAnalysis is the complete LLM analysis of a book chapter
type ScriptAnalysis struct {
	Language   string          `json:"language"`
	Characters []Character     `json:"characters"`
	Script     []ScriptSegment `json:"script"`
	BGMTimeline []BGMCue       `json:"bgm_timeline"`
}

// Character identified by LLM
type Character struct {
	Name        string `json:"name"`
	Role        string `json:"role"`        // protagonist, antagonist, narrator, supporting
	Gender      string `json:"gender"`      // male, female, neutral
	Age         string `json:"age"`         // young, middle-aged, elderly
	Personality string `json:"personality"` // calm, hot-tempered, gentle, cunning...
	VoiceDesign string `json:"voice_design"` // LLM-generated voice design prompt for TTS
}

// ScriptSegment is one piece of the script timeline
type ScriptSegment struct {
	Index       int    `json:"index"`
	Type        string `json:"type"`        // narration, dialogue
	Speaker     string `json:"speaker"`     // character name, empty for narration
	Text        string `json:"text"`
	Emotion     string `json:"emotion"`     // happy, sad, angry, fearful, calm, excited...
	EmotionHint string `json:"emotion_hint"` // TTS style directive in natural language
	Scene       string `json:"scene"`        // normal, phone_call, inner_monologue, underwater, broadcast, thinking
	SFX         []SFXCue `json:"sfx"`       // sound effects to play during this segment
}

// SFXCue describes a sound effect insertion point
type SFXCue struct {
	Keyword string  `json:"keyword"`  // matched sound effect keyword
	Timing  string  `json:"timing"`   // before, during, after
	Offset  float64 `json:"offset"`   // seconds offset within segment
	Reason  string  `json:"reason"`   // why this SFX was chosen
}

// BGMCue describes background music timing
type BGMCue struct {
	SegmentStart int    `json:"segment_start"` // segment index
	SegmentEnd   int    `json:"segment_end"`
	Emotion      string `json:"emotion"`       // dominant emotion in this range
	BGMKeyword   string `json:"bgm_keyword"`   // keyword to match BGM
	Action       string `json:"action"`        // start, stop, crossfade
}

// AnalyzeChapter analyzes one chapter with LLM
func (a *Analyzer) AnalyzeChapter(ctx context.Context, chapter model.Chapter, knownCharacters []Character) (*ScriptAnalysis, error) {
	systemPrompt := `You are an expert audiobook director. Analyze the provided novel text.
CRITICAL: Output ONLY a single valid JSON object. No markdown fences, no extra text, no explanations — JUST the JSON object.

The JSON must have these fields:
1. "language": detected language (zh-CN, en-US, etc.)
2. "characters": list of {name, role, gender, age, personality, voice_design}
   - role: narrator, protagonist, antagonist, supporting
   - voice_design: vivid 2-4 sentence voice description for TTS (age, gender, tone, pace, texture, emotion)
3. "script": array of {index, type, speaker, text, emotion, emotion_hint, scene, sfx}
   - type: "narration" or "dialogue"
   - speaker: character name exactly as in characters list
   - emotion: emotion with intensity (e.g., "excited_high", "sad_mild", "angry_intense")
   - emotion_hint: natural language TTS style directive in source language
   - scene: "normal", "phone_call", "inner_monologue", "underwater", "broadcast", or "mecha_voice"
   - sfx: array of {keyword, timing(before|during|after), offset(seconds), reason}
4. "bgm_timeline": array of {segment_start, segment_end, emotion, bgm_keyword, action(start|stop|crossfade)}

WRITE emotion_hint AND voice_design IN THE SAME LANGUAGE AS THE SOURCE TEXT.`

	textPreview := chapter.Content
	if len(textPreview) > 8000 {
		textPreview = textPreview[:8000] + "\n...(truncated)"
	}

	var analysis ScriptAnalysis
	if err := a.client.ChatJSON(ctx, systemPrompt, textPreview, &analysis); err != nil {
		return nil, fmt.Errorf("llm analysis: %w", err)
	}

	// Merge with known characters to maintain consistency
	analysis = mergeCharacters(analysis, knownCharacters)

	return &analysis, nil
}

// mergeCharacters merges new characters with previously known ones
func mergeCharacters(new ScriptAnalysis, known []Character) ScriptAnalysis {
	knownByName := make(map[string]Character)
	for _, c := range known {
		knownByName[strings.ToLower(c.Name)] = c
	}

	for i, c := range new.Characters {
		key := strings.ToLower(c.Name)
		if existing, ok := knownByName[key]; ok {
			// Keep existing voice_design for consistency
			if existing.VoiceDesign != "" {
				new.Characters[i].VoiceDesign = existing.VoiceDesign
			}
		} else {
			knownByName[key] = c
		}
	}

	// Fix speaker references in script
	for i, seg := range new.Script {
		if seg.Type == "dialogue" {
			key := strings.ToLower(seg.Speaker)
			if c, ok := knownByName[key]; ok {
				new.Script[i].Speaker = c.Name
			}
		}
	}

	return new
}

// BuildVoiceProfiles converts LLM character analysis to voice profiles
func (a *Analyzer) BuildVoiceProfiles(characters []Character) []model.VoiceProfile {
	var profiles []model.VoiceProfile
	for _, c := range characters {
		if c.Role == "narrator" || c.VoiceDesign == "" {
			continue
		}
		profiles = append(profiles, model.VoiceProfile{
			Name:         c.Name,
			Source:       "design",
			Engine:       "mimo",
			DesignPrompt: c.VoiceDesign,
			Description:  fmt.Sprintf("%s, %s, %s. %s", c.Role, c.Gender, c.Age, c.Personality),
			Language:     "zh-CN",
			Gender:       c.Gender,
		})
	}
	return profiles
}

// CompileSpeechSegments extracts all speech segments from the analysis
func CompileSpeechSegments(analysis ScriptAnalysis) []SpeechSegment {
	var segments []SpeechSegment
	for _, seg := range analysis.Script {
		segments = append(segments, SpeechSegment{
			Index:       seg.Index,
			Type:        seg.Type,
			Speaker:     seg.Speaker,
			Text:        seg.Text,
			Emotion:     seg.Emotion,
			EmotionHint: seg.EmotionHint,
			Scene:       seg.Scene,
			SFX:         seg.SFX,
		})
	}
	return segments
}

// SpeechSegment is a simplified segment for the synthesis pipeline
type SpeechSegment struct {
	Index       int      `json:"index"`
	Type        string   `json:"type"`
	Speaker     string   `json:"speaker"`
	Text        string   `json:"text"`
	Emotion     string   `json:"emotion"`
	EmotionHint string   `json:"emotion_hint"`
	Scene       string   `json:"scene"`
	SFX         []SFXCue `json:"sfx"`
}
