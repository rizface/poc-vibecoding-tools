package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"google.golang.org/genai"
)

var client *genai.Client
var model = "gemini-2.5-pro"

var generateCodeSystemInstruction = &genai.Content{
	Parts: []*genai.Part{
		{
			Text: `
				System Role: Act as a dual-role Lead Designer and Senior Frontend Engineer from a MAANG company. You specialize in high-end, modern, and clean UI/UX and efficient code diffing.
				Task: Based on the User Request and the provided Technical Specification, generate or update the web project.
				Technical Constraints:
					- File Strategy: You are strictly limited to .html, .css, and .js extensions.
					- Incremental Logic: You will be provided with a <file_tree> of existing files.
						- If the <file_tree> is empty, generate the full project.
						- If the <file_tree> is not empty, you must only return the files that require changes or new files that need to be created. Do not return unchanged files.
					- Styling: Use Tailwind CSS via the official Play CDN script in the <head> of the index.html.
					- Design Aesthetic: Follow "Modern & Clean" principles—heavy focus on ample whitespace, refined typography (Inter/Geist), and a professional color palette.
					- Code Standards: Write modular Vanilla ES6+ JavaScript. Ensure full accessibility (WCAG compliant).
				Output Requirement:
					- Respond exclusively with a valid JSON object. No markdown backticks, no introductory text, and no prose.
				Format: {"contents": [{"filename": string, "code": string}]}
			`,
		},
	},
}

var generateCodeStreamConfig = &genai.GenerateContentConfig{
	Temperature:       genai.Ptr[float32](0.2),
	ResponseMIMEType:  "application/json",
	SystemInstruction: generateCodeSystemInstruction,
}

type GenAIResponse struct {
	Filename string `json:"filename"`
	Code     string `json:"code"`
}

var generateChatSystemInstruction = &genai.Content{
	Parts: []*genai.Part{
		{
			Text: `
			You are the Lead Project Architect. Your goal is to act as a bridge between the user's vision and the technical implementation team. You must ensure that every detail of the web project is clear before code generation begins.
			Your Context:
				- Chat History: You will receive the conversation between the AI and the User.
				- File Context: You will see the current state of any existing files in the project.
			Your Objectives:
				- Clarify Vision: If the user's request is vague (e.g., "make a dashboard"), ask targeted questions about features, color schemes, or specific sections.
				- Analyze Progress: If files already exist, determine if the new request requires a simple update or a major structural change.
				- Gatekeeping: Do not set readyToExecute to true until you have a clear understanding of:
					- The primary purpose of the page.
					- The specific sections or components needed.
					- The desired visual style (e.g., "Glassmorphism," "Corporate Clean," "Cyberpunk").
			Output Requirement:
			You must respond exclusively with a valid JSON object.
				- readyToExecute: Set to true ONLY when you have enough detail to pass to the Requirement Generator. Set to false if you need to ask the user more questions.
				- response: If readyToExecute is false, this should be a helpful, conversational message to the user asking for missing info. If true, this should be a brief confirmation (e.g., "Got it! Starting the build now.").
				Format:
				{"readyToExecute": boolean, "response": string}
			`,
		},
	},
}

var generateChatStreamConfig = &genai.GenerateContentConfig{
	Temperature:       genai.Ptr[float32](0.2),
	ResponseMIMEType:  "application/json",
	SystemInstruction: generateChatSystemInstruction,
}

type GenAIResponseChatStream struct {
	ReadyToExecute bool   `json:"readyToExecute"`
	Response       string `json:"response"`
}

var generateRequirementystemInstruction = &genai.Content{
	Parts: []*genai.Part{
		{
			Text: `
			You are a Senior Technical Architect at a MAANG company. Your role is to transform user conversations and existing codebases into a precise, actionable Technical Specification.
			Your Context:
				- Chat History: The full context of what the user wants, including any clarifications made by the Planner.
				- Files: The current state of the project’s source code.
			Your Task:
				- Create a comprehensive "Technical Specification" that acts as the single source of truth for the implementation phase. You must detail:
				- Visual Architecture: Layout structure, spacing constants (using Tailwind scales), and color palette (hex codes or Tailwind classes).
				- Component Breakdown: A list of UI components and their specific behaviors.
				- Functional Logic: Specific JavaScript logic, state management requirements, and API interactions (if any).
				- File Manifest: Explicitly list which files need to be created or modified.
				- Edge Cases: Define how the UI should handle loading, empty states, or responsiveness.
			Rules for Output:
				- File Strategy: You are strictly limited to .html, .css, and .js extensions.
				- Be Specific: Do not say "Modern design." Say "Use a sticky header with a backdrop-blur-md, Slate-900 background, and Zinc-400 text."
				- Focus on Changes: If files exist, explicitly describe what needs to change in the existing code vs. what is a new addition.
				- No Code: Do not write the actual code. Describe the logic and structure so the Implementer can write it.
			Format:{
				"specification": "string (Markdown formatted technical document)",
				"estimatedComplexity": "Low | Medium | High",
				"changeType": "Initial Build | Feature Update | Bug Fix"
			}
			`,
		},
	},
}

var generateRequirementConfig = &genai.GenerateContentConfig{
	Temperature:       genai.Ptr[float32](0.2),
	ResponseMIMEType:  "application/json",
	SystemInstruction: generateRequirementystemInstruction,
}

type GenAIResponseRequirement struct {
	Spec       string `json:"specification"`
	Complexity string `json:"estimatedComplexity"`
	ChangeType string `json:"changeType"`
}

func buildChatHistoryForPlanner(histories []ChatHistoryModel) (string, error) {
	type chat struct {
		LLM  string `json:"llm"`
		User string `json:"user"`
	}

	chats := []chat{}
	for _, v := range histories {
		chats = append(chats, chat{
			LLM:  v.Response,
			User: v.Chat,
		})
	}

	historyJson, err := json.Marshal(chats)
	if err != nil {
		return "", err
	}

	return string(historyJson), nil
}

func buildFileTreeForPlanner(files []ProjectFileModel) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	type content struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	contents := []content{}
	for _, v := range files {
		filePath := fmt.Sprintf("%s/code-generation/%s/%s", homeDir, v.ProjectID, v.Path)

		file, err := os.Open(filePath)
		if err != nil {
			return "", err
		}

		var code strings.Builder
		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			code.WriteString(string(line))

			if err != nil {
				if err == io.EOF {
					break
				}

				log.Fatal(err)
			}
		}

		contents = append(contents, content{
			Path:    v.Path,
			Content: code.String(),
		})
	}

	contentsJson, err := json.Marshal(contents)
	if err != nil {
		return "", err
	}

	return string(contentsJson), nil
}

func streamCodeGeneration(ctx context.Context, spec GenAIResponseRequirement, files []ProjectFileModel) (chan string, error) {
	codeStream := make(chan string)

	filesPrompt, err := buildFileTreeForPlanner(files)
	if err != nil {
		return nil, err
	}

	if len(filesPrompt) == 0 {
		filesPrompt = "Empty (New Project)"
	}

	prompt := fmt.Sprintf(`
		<technical_specification>
		%s
		</technical_specification>

		<file_tree>
		%s
		</file_tree>

		Generate the necessary code following the incremental logic instructions.
	`, spec.Spec, filesPrompt)

	iter := client.Models.GenerateContentStream(
		ctx,
		model,
		genai.Text(prompt),
		generateCodeStreamConfig,
	)

	go func() {
		for resp, err := range iter {
			if err != nil {
				fmt.Printf("[ERROR]: generate code %s \n", err.Error())
				break
			}

			for _, candidate := range resp.Candidates {
				for _, part := range candidate.Content.Parts {
					codeStream <- part.Text
				}
			}
		}

		close(codeStream)
	}()

	return codeStream, nil
}

func streamChat(ctx context.Context, projectId string, chatHistories []ChatHistoryModel, files []ProjectFileModel) (chan string, error) {
	chatStream := make(chan string)

	chatHistoryPrompt, err := buildChatHistoryForPlanner(chatHistories)
	if err != nil {
		return nil, err
	}

	filesPrompt, err := buildFileTreeForPlanner(files)
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`
		// Chat History
		%s

		// Files
		%s

		Analyze the above and determine if we are ready to build.
	`, chatHistoryPrompt, filesPrompt)

	iter := client.Models.GenerateContentStream(
		ctx,
		model,
		genai.Text(prompt),
		generateChatStreamConfig,
	)

	go func() {
		for resp, err := range iter {
			if err != nil {
				fmt.Printf("[ERROR]: generate chat %s \n", err.Error())
				break
			}

			for _, candidate := range resp.Candidates {
				for _, part := range candidate.Content.Parts {
					chatStream <- part.Text
				}
			}
		}

		close(chatStream)
	}()

	return chatStream, nil
}

func streamRequirement(ctx context.Context, chatHistories []ChatHistoryModel, files []ProjectFileModel) (chan string, error) {
	reqStream := make(chan string)

	chatHistoryPrompt, err := buildChatHistoryForPlanner(chatHistories)
	if err != nil {
		return nil, err
	}

	filesPrompt, err := buildFileTreeForPlanner(files)
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`
		<chat_history>
		%s
		<chat_history>

		<file_structure>
		%s
		<file_structure>

		Based on the chat history and existing files, generate a detailed Technical Specification JSON.
	`, chatHistoryPrompt, filesPrompt)
	fmt.Println(prompt)

	iter := client.Models.GenerateContentStream(
		ctx,
		model,
		genai.Text(prompt),
		generateRequirementConfig,
	)

	go func() {
		for resp, err := range iter {
			if err != nil {
				fmt.Printf("[ERROR]: generate requirement %s \n", err.Error())
				break
			}

			for _, candidate := range resp.Candidates {
				for _, part := range candidate.Content.Parts {
					reqStream <- part.Text
				}
			}
		}

		close(reqStream)
	}()

	return reqStream, nil
}
