package doctor

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/PullRequestInc/go-gpt3"

	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/pluginx"
)

const (
	PluginName     = "doctor"
	promptTemplate = "Can you show me 3 possible kubectl commands to take an action after resource '%s' in namespace '%s' (if namespace needed) fails with error '%s'?"
)

var (
	k8sPromptRegex = regexp.MustCompile(`--(\w+)=([^\s]+)`)
)

type Config struct {
	ApiKey string `yaml:"apiKey"`
}

// Executor provides functionality for running Doctor.
type Executor struct {
	pluginVersion string
	gptClient     gpt3.Client
	l             sync.Mutex
}

// NewExecutor returns a new Executor instance.
func NewExecutor(ver string) *Executor {
	return &Executor{
		pluginVersion: ver,
	}
}

// Metadata returns details about the Doctor plugin.
func (d *Executor) Metadata(context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:     "1.0.0",
		Description: "Doctor helps in finding the root cause of a k8s problem.",
		JSONSchema: api.JSONSchema{
			Value: heredoc.Doc(`{
				  "$schema": "http://json-schema.org/draft-04/schema#",
				  "title": "doctor",
				  "description": "Doctor helps in finding the root cause of a k8s problem.",
				  "type": "object",
				  "properties": {
					"apiKey": {
					  "description": "Open API Key",
					  "type": "string"
					}
				  },
				  "additionalProperties": false
				}`),
		},
	}, nil
}

// Execute returns a given command as a response.
func (d *Executor) Execute(ctx context.Context, in executor.ExecuteInput) (executor.ExecuteOutput, error) {
	var cfg Config
	err := pluginx.MergeExecutorConfigs(in.Configs, &cfg)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while merging input configuration: %w", err)
	}
	doctorParams, err := normalizeCommand(in.Command)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while normalizing command: %w", err)
	}
	gpt, err := d.getGptClient(&cfg)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while initializing GPT client: %w", err)
	}
	sb := strings.Builder{}
	err = gpt.CompletionStreamWithEngine(ctx,
		gpt3.TextDavinci003Engine,
		gpt3.CompletionRequest{
			Prompt:      []string{buildPrompt(doctorParams)},
			MaxTokens:   gpt3.IntPtr(300),
			Temperature: gpt3.Float32Ptr(0),
		}, func(resp *gpt3.CompletionResponse) {
			text := resp.Choices[0].Text
			sb.WriteString(text)
		})
	if err != nil {
		return executor.ExecuteOutput{}, err
	}
	response := sb.String()
	response = strings.TrimLeft(response, "\n")
	if doctorParams.IsRaw() {
		return executor.ExecuteOutput{
			Message: api.NewPlaintextMessage(response, true),
		}, nil
	}
	btnBuilder := api.NewMessageButtonBuilder()
	var btns []api.Button
	for i, s := range strings.Split(response, "\n") {
		parts := strings.Split(s, "")
		if len(parts) < 4 {
			continue
		}
		s = strings.Join(parts[3:], "")
		btns = append(btns, btnBuilder.ForCommandWithDescCmd(fmt.Sprintf("Choice %d", i+1), s, api.ButtonStylePrimary))
	}
	return executor.ExecuteOutput{
		Message: api.Message{
			BaseBody: api.Body{
				Plaintext: "Possible actions",
			},
			Sections: []api.Section{
				{
					Buttons: btns,
				},
			},
			OnlyVisibleForYou: false,
			ReplaceOriginal:   false,
		},
	}, nil
}

// Help returns help message
func (d *Executor) Help(context.Context) (api.Message, error) {
	btnBuilder := api.NewMessageButtonBuilder()
	return api.Message{
		Sections: []api.Section{
			{
				Base: api.Base{
					Header:      "Run `doctor` commands",
					Description: "Doctor helps in finding the root cause of a k8s problem.",
				},
				Buttons: []api.Button{
					btnBuilder.ForCommandWithDescCmd("Run", "doctor 'text'"),
				},
			},
		},
	}, nil
}

func (d *Executor) getGptClient(cfg *Config) (gpt3.Client, error) {
	d.l.Lock()
	defer d.l.Unlock()
	if cfg.ApiKey == "" {
		return nil, fmt.Errorf("OpenAPI API Key cannot be empty. You generate it here: https://platform.openai.com/account/api-keys")
	}
	if d.gptClient == nil {
		d.gptClient = gpt3.NewClient(cfg.ApiKey)
	}
	return d.gptClient, nil
}

type DoctorParams struct {
	RawText   string
	Resource  string
	Namespace string
	Error     string
}

func (p *DoctorParams) IsRaw() bool {
	return p.Resource == "" || p.Error == ""
}
func normalizeCommand(command string) (DoctorParams, error) {
	matches := k8sPromptRegex.FindAllStringSubmatch(command, -1)
	params := DoctorParams{}
	params.RawText = command
	for _, match := range matches {
		if len(match) != 3 {
			return DoctorParams{}, errors.New("invalid command")
		}
		key := match[1]
		value := match[2]

		switch key {
		case "resource":
			params.Resource = value
		case "namespace":
			params.Namespace = value
		case "error":
			params.Error = value
		}
	}
	return params, nil
}

func buildPrompt(p DoctorParams) string {
	if p.IsRaw() {
		return p.RawText
	}
	return fmt.Sprintf(promptTemplate, p.Resource, p.Namespace, p.Error)
}
