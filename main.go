package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"
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
		log.Warn("Warning: .env file not found.")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set in environment variables.")
	}

	inputDirPath := flag.String("input", "", "Path to the input directory")
	outputDirPath := flag.String("output", "output", "Path to the output directory")
	flag.Parse()

	if *inputDirPath == "" {
		log.Fatal("Please specify an input directory path using the --input flag.")
	}

	if err := os.MkdirAll(*outputDirPath, os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %s", err)
	}

	log.Info("Starting processing...")

	files, err := ioutil.ReadDir(*inputDirPath)
	if err != nil {
		log.Fatalf("Failed to read input directory: %s", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		inputFilePath := filepath.Join(*inputDirPath, file.Name())
		log.Infof("Processing file: %s", inputFilePath)

		content, err := ioutil.ReadFile(inputFilePath)
		if err != nil {
			log.Errorf("Failed to read input file: %s", err)
			continue
		}

		var snippetLabelPairs []SnippetLabelPair
		chunks := splitText(string(content), maxTokens)
		for _, chunk := range chunks {
			log.Info("Getting snippet label pairs...")
			pairs := processText(chunk, apiKey)
			snippetLabelPairs = append(snippetLabelPairs, pairs...)
		}

		outputData, err := json.MarshalIndent(snippetLabelPairs, "", "  ")
		if err != nil {
			log.Errorf("Failed to marshal data into JSON: %s", err)
			continue
		}

		outputFileName := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())) + ".json"
		outputFilePath := filepath.Join(*outputDirPath, outputFileName)

		if err := ioutil.WriteFile(outputFilePath, outputData, 0644); err != nil {
			log.Errorf("Failed to write output to file: %s", err)
			continue
		}

		log.Infof("Output successfully written to %s", outputFilePath)
	}

	log.Info("Processing completed.")
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

	log.Debugf("OpenAI API Response: %s", string(body))

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
		log.Warn("No choices were returned by OpenAI.")
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
