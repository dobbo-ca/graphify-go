package semantic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
	"github.com/dobbo-ca/graphify-go/internal/model"
)

// bedrockModel is Claude Opus 4.8 on Amazon Bedrock — the Mantle path takes the
// anthropic. provider prefix on the model id.
const bedrockModel = "anthropic." + string(anthropic.ModelClaudeOpus4_8)

// ambiguousScore is the confidence-score floor below which a semantic edge is
// recorded AMBIGUOUS rather than INFERRED, so weak inferences are flagged for
// review in the report instead of presented as confident.
const ambiguousScore = 0.5

// conceptTool is the single tool the model must call: it returns the concepts a
// note is about and how each relates to the note. strict schema validation
// guarantees the input parses.
var conceptTool = anthropic.ToolParam{
	Name:        "record_concepts",
	Description: anthropic.String("Record the key concepts this engineering/design note is about and how the note relates to each, for building a knowledge graph."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"concepts": map[string]any{
				"type":        "array",
				"description": "The concepts the note discusses. 1-6 of the most central concepts.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "relation", "score"},
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Short canonical concept name (e.g. \"AppConfig\", \"Flux\", \"forge\", \"build environment\", \"CI dependency management\").",
						},
						"relation": map[string]any{
							"type":        "string",
							"enum":        []string{"cites", "conceptually_related_to", "semantically_similar_to"},
							"description": "How the note relates to the concept.",
						},
						"score": map[string]any{
							"type":        "number",
							"description": "Confidence the note is genuinely about this concept, 0.0-1.0.",
						},
					},
				},
			},
		},
		Required: []string{"concepts"},
	},
}

const systemPrompt = `You extract the key concepts an engineering or design note is about, for a knowledge graph that links related notes.
Given one note, identify the 1-6 most central concepts (systems, tools, services, or topics — e.g. "AppConfig", "Flux", "forge", "build environment", "CI dependency management").
Prefer concept names that other notes about the same system would also use, so notes about the same topic converge on the same concept.
Call the record_concepts tool exactly once. Do not invent concepts the note does not discuss.`

// concept mirrors one item the model returns through the record_concepts tool.
type concept struct {
	Name     string  `json:"name"`
	Relation string  `json:"relation"`
	Score    float64 `json:"score"`
}

type conceptsPayload struct {
	Concepts []concept `json:"concepts"`
}

// BedrockBackend is a Backend backed by Anthropic Claude on Amazon Bedrock.
type BedrockBackend struct {
	client *bedrock.MantleClient
	model  string
}

// NewBedrockBackend builds a Bedrock-backed concept extractor for the given AWS
// region, resolving credentials through the standard AWS chain (AWS_PROFILE,
// env, instance role). region defaults to us-east-1 when empty.
func NewBedrockBackend(ctx context.Context, region string) (*BedrockBackend, error) {
	if region == "" {
		region = "us-east-1"
	}
	c, err := bedrock.NewMantleClient(ctx, bedrock.MantleClientConfig{AWSRegion: region})
	if err != nil {
		return nil, fmt.Errorf("bedrock: %w", err)
	}
	return &BedrockBackend{client: c, model: bedrockModel}, nil
}

func (b *BedrockBackend) Name() string { return "bedrock" }

// Extract asks Claude to record the concepts the note is about and turns the
// tool call into concept nodes and edges. A note with no usable tool call
// yields nothing rather than an error.
func (b *BedrockBackend) Extract(ctx context.Context, n Note) ([]model.Node, []model.Edge, error) {
	resp, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &conceptTool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: conceptTool.Name},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(notePrompt(n))),
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("bedrock invoke: %w", err)
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == conceptTool.Name {
			return parseConcepts(n, []byte(tu.JSON.Input.Raw()))
		}
	}
	return nil, nil, nil
}

// notePrompt frames one note for the model, capping the body defensively (the
// caller already enforces MaxNoteBytes).
func notePrompt(n Note) string {
	body := n.Content
	if len(body) > MaxNoteBytes {
		body = body[:MaxNoteBytes]
	}
	return fmt.Sprintf("Note path: %s\n\n%s", n.File, body)
}

// parseConcepts converts the record_concepts tool input into concept nodes and
// edges from the note. Each concept becomes a stable concept node (id via
// idutil, so notes naming the same concept converge on one node) and an edge
// from the note carrying the relation and score. Unknown relations are dropped;
// low-score edges are marked AMBIGUOUS. Malformed JSON is an error so the caller
// skips the note.
func parseConcepts(n Note, raw []byte) ([]model.Node, []model.Edge, error) {
	var p conceptsPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, nil, fmt.Errorf("bedrock: bad tool output: %w", err)
	}
	var nodes []model.Node
	var edges []model.Edge
	seen := map[string]bool{}
	for _, c := range p.Concepts {
		if c.Name == "" || !validRelations[c.Relation] {
			continue
		}
		id := idutil.MakeID("concept", c.Name)
		if id == "" {
			continue
		}
		if !seen[id] {
			seen[id] = true
			nodes = append(nodes, model.Node{ID: id, Label: c.Name, FileType: ConceptType})
		}
		conf := "INFERRED"
		if c.Score < ambiguousScore {
			conf = "AMBIGUOUS"
		}
		edges = append(edges, model.Edge{
			Source: n.ID, Target: id, Relation: c.Relation,
			Confidence: conf, ConfidenceScore: c.Score,
		})
	}
	return nodes, edges, nil
}
