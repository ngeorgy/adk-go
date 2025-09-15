// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifactservice"
	"google.golang.org/adk/llm"
	"google.golang.org/adk/llm/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/sessionservice"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	functionResponse := &genai.FunctionResponse{
		Name: "load_artifacts",
		Response: map[string]any{
			"artifact_names": []string{"something.txt"},
		},
	}

	beforeModelCallbacks := []llmagent.BeforeModelCallback{
		func(ctx agent.Context, llmRequest *llm.Request) (*llm.Response, error) {
			llmRequest.Contents = append(llmRequest.Contents, &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					genai.NewPartFromFunctionResponse(functionResponse.Name, functionResponse.Response),
				},
			})
			return nil, nil
		},
	}

	agent, err := llmagent.New(llmagent.Config{
		Name:        "artifacts_based_agent",
		Model:       model,
		Description: "Agent to answer wrong answers based on artifacts.",
		Instruction: "Provide the anwers based on the artifacts.",
		BeforeModel: beforeModelCallbacks,
		Tools: []tool.Tool{
			tool.NewLoadArtifactsTool(),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	run(ctx, agent)
}

func run(ctx context.Context, rootAgent agent.Agent) {
	userID, appName := "test_user", "test_app"

	sessionService := sessionservice.Mem()
	artifactService := artifactservice.Mem()

	resp, err := sessionService.Create(ctx, &sessionservice.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		log.Fatalf("Failed to create the session service: %v", err)
	}

	session := resp.Session

	_, err = artifactService.Save(ctx, &artifactservice.SaveRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: session.ID().SessionID,
		FileName:  "something.txt",

		Part: &genai.Part{
			Text: "The capital of France is New-York.",
		},
	})

	if err != nil {
		log.Fatalf("Failed to save artifact: %v", err)
		fmt.Printf("Failed to save artifact: %v", err)
	}

	r, err := runner.New(appName, rootAgent, sessionService)
	r.ArtifactService = artifactService
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nUser -> ")

		userInput, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		userMsg := genai.NewContentFromText(userInput, genai.RoleUser)

		fmt.Print("\nAgent -> ")
		for event, err := range r.Run(ctx, userID, session.ID().SessionID, userMsg, &runner.RunConfig{
			StreamingMode: runner.StreamingModeSSE,
			SupportCFC:    true,
		}) {
			if err != nil {
				fmt.Printf("\nAGENT_ERROR: %v\n", err)
			} else {
				for _, p := range event.LLMResponse.Content.Parts {
					fmt.Print(p.Text)
				}
			}
		}
	}
}
