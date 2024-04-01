package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
)

type SnippetLabelPair struct {
	Label   string `json:"label"`
	Snippet string `json:"snippet"`
}

const (
	maxTokens = 2000
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found.")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set in environment variables.")
	}

	inputFilePath := flag.String("input", "", "Path to the input text file")
	flag.Parse()

	if *inputFilePath == "" {
		log.Fatal("Please specify an input file path using the --input flag.")
	}

	content, err := ioutil.ReadFile(*inputFilePath)
	if err != nil {
		log.Fatalf("Failed to read input file: %s", err)
	}

	var snippetLabelPairs []SnippetLabelPair
	chunks := splitText(string(content), maxTokens)
	for _, chunk := range chunks {
		pairs := processText(chunk, apiKey)
		snippetLabelPairs = append(snippetLabelPairs, pairs...)
	}

	outputData, err := json.MarshalIndent(snippetLabelPairs, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal data into JSON: %s", err)
	}

	fmt.Println(string(outputData))

	if err := ioutil.WriteFile("output.json", outputData, 0644); err != nil {
		log.Fatalf("Failed to write output to file: %s", err)
	}

	log.Println("Output successfully written to output.json")
}

func processText(text, apiKey string) []SnippetLabelPair {
	return generateSnippetLabelPairs(text, apiKey)
}

func generateSnippetLabelPairs(text, apiKey string) []SnippetLabelPair {
	client := &http.Client{}
	prompt := fmt.Sprintf("Please read the following text and generate an array of label/snippet objects. Each object should contain a concise label for the main theme or idea discussed in the snippet, along with the corresponding snippet of text:\n\n\"%s\"", text)

	requestBody, err := json.Marshal(map[string]interface{}{
		"model":       "gpt-3.5-turbo",
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.7,
		"max_tokens":  2000,
		"top_p":       1.0,
		"n":           1,
	})
	if err != nil {
		log.Fatalf("Error marshaling request body: %s", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(requestBody))
	if err != nil {
		log.Fatalf("Error creating request: %s", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error making request to OpenAI: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %s", err)
	}

	fmt.Println(string(body))

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		log.Fatalf("Error unmarshaling response: %s", err)
	}

	if len(response.Choices) == 0 {
		log.Println("No choices were returned by OpenAI.")
		return []SnippetLabelPair{}
	}

	responseContent := response.Choices[0].Message.Content
	responseContent = strings.ReplaceAll(responseContent, "\\n", "")
	responseContent = strings.ReplaceAll(responseContent, "\\\"", "\"")
	responseContent = strings.TrimSpace(responseContent)

	var snippetLabelPairs []SnippetLabelPair
	regex := regexp.MustCompile(`{[\s\n]*"label":\s*"(.*?)"[\s\n]*,[\s\n]*"snippet":\s*"(.*?)"[\s\n]*}`)
	matches := regex.FindAllStringSubmatch(responseContent, -1)
	for _, match := range matches {
		if len(match) == 3 {
			label := match[1]
			snippet := match[2]
			pair := SnippetLabelPair{
				Label:   label,
				Snippet: snippet,
			}
			snippetLabelPairs = append(snippetLabelPairs, pair)
		}
	}

	return snippetLabelPairs
}

func splitText(text string, maxTokens int) []string {
	var chunks []string
	words := strings.Fields(text)
	currentChunk := ""

	for _, word := range words {
		if len(currentChunk)+len(word)+1 > maxTokens {
			chunks = append(chunks, currentChunk)
			currentChunk = ""
		}
		currentChunk += " " + word
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}
