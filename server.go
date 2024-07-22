package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-github/v63/github"
	"k8s.io/client-go/kubernetes"
)

type server struct {
	client           *kubernetes.Clientset
	githubClient     *github.Client
	webhookSecretKey string
}

func (s server) webhook(w http.ResponseWriter, req *http.Request) {
	ctx := context.Background()
	payload, err := github.ValidatePayload(req, []byte(s.webhookSecretKey))
	if err != nil {
		w.WriteHeader(500)
		fmt.Printf("validate Payload error: %v", err)
		return
	}
	event, err := github.ParseWebHook(github.WebHookType(req), payload)
	if err != nil {
		w.WriteHeader(500)
		fmt.Printf("validate Payload error: %v", err)
		return
	}
	switch event := event.(type) {
	case *github.Hook:
		fmt.Println("hook created")
	case *github.PushEvent:
		files := getFiles(event.Commits)
		for _, filename := range files {
			dowloadedFile, _, err := s.githubClient.Repositories.DownloadContents(ctx, *event.Repo.Owner.Name, *event.Repo.Name, filename, &github.RepositoryContentGetOptions{})
			if err != nil {
				w.WriteHeader(500)
				fmt.Printf("DownloadContents error: %v", err)
				return
			}

			defer dowloadedFile.Close()
			fileBody, err := io.ReadAll(dowloadedFile)
			if err != nil {
				w.WriteHeader(500)
				fmt.Printf("Read Downloaded file error: %v", err)
				return
			}

			_, _, err = deploy(ctx, *s.client, fileBody)
			if err != nil {
				w.WriteHeader(500)
				fmt.Printf("Deploy error: %v", err)
				return
			}
		}

	default:
		w.WriteHeader(500)
		fmt.Printf("event not found: %v", event)
		return
	}
}

func getFiles(commits []*github.HeadCommit) []string {
	allFiles := []string{}
	for _, commit := range commits {
		allFiles = append(allFiles, commit.Added...)
		allFiles = append(allFiles, commit.Modified...)
	}

	allUniqueFiles := make(map[string]bool)
	for _, filename := range allFiles {
		allUniqueFiles[filename] = true
	}

	allUniqueFilesSlice := []string{}
	for filename := range allUniqueFiles {
		allUniqueFilesSlice = append(allUniqueFilesSlice, filename)
	}
	return allUniqueFilesSlice
}
