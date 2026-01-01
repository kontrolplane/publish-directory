package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Config struct {
	Repository       string `env:"INPUT_REPOSITORY"`
	Branch           string `env:"INPUT_BRANCH"`
	Folder           string `env:"INPUT_FOLDER"`
	CommitUser       string `env:"INPUT_COMMIT_USERNAME" envDefault:"github-actions[bot]"`
	CommitEmail      string `env:"INPUT_COMMIT_EMAIL" envDefault:"github-actions[bot]@users.noreply.github.com"`
	CommitMessage    string `env:"INPUT_COMMIT_MESSAGE" envDefault:"chore: update branch from directory"`
	GithubToken      string `env:"GITHUB_TOKEN"`
	GithubRepository string `env:"GITHUB_REPOSITORY"`
}

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	if err := validateConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if err := publishDirectory(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully published directory to branch")
}

func loadConfig() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validateConfig(cfg Config) error {
	if _, err := os.Stat(cfg.Folder); os.IsNotExist(err) {
		return fmt.Errorf("folder '%s' does not exist", cfg.Folder)
	}

	return nil
}

func publishDirectory(cfg Config) error {
	repository := cfg.Repository
	if repository == "" {
		var err error
		repository, err = getCurrentRepository()
		if err != nil {
			return fmt.Errorf("failed to determine repository: %w", err)
		}
	}

	temporaryDirectory, err := os.MkdirTemp("", "kontrolplane-publish-directory-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	url := fmt.Sprintf("https://github.com/%s.git", repository)

	auth := &http.BasicAuth{
		Username: "x-access-token",
		Password: cfg.GithubToken,
	}

	repo, err := cloneOrCreateBranch(url, cfg.Branch, temporaryDirectory, auth)
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if err := cleanWorkingTree(temporaryDirectory); err != nil {
		return fmt.Errorf("failed to clean working tree: %w", err)
	}

	if err := copyDirectory(cfg.Folder, temporaryDirectory); err != nil {
		return fmt.Errorf("failed to copy directory: %w", err)
	}

	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if status.IsClean() {
		fmt.Println("No changes to commit")
		return nil
	}

	commit, err := worktree.Commit(cfg.CommitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  cfg.CommitUser,
			Email: cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("Created commit: %s\n", commit.String())

	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		Progress:   os.Stdout,
	}); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

func getCurrentRepository() (string, error) {
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return "", fmt.Errorf("GITHUB_REPOSITORY environment variable not set")
	}
	return repo, nil
}

func cloneOrCreateBranch(gitURL, branch string, targetDir string, auth *http.BasicAuth) (*git.Repository, error) {
	branchReference := plumbing.NewBranchReferenceName(branch)
	repo, err := git.PlainClone(targetDir, false, &git.CloneOptions{
		URL:           gitURL,
		Auth:          auth,
		ReferenceName: branchReference,
		SingleBranch:  true,
		Depth:         1,
	})
	if err == nil {
		return repo, nil
	}

	fmt.Printf("Branch '%s' doesn't exist, creating new orphan branch\n", branch)

	repo, err = git.PlainInit(targetDir, false)
	if err != nil {
		return nil, fmt.Errorf("failed to init repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchReference,
		Create: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{gitURL},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add remote: %w", err)
	}

	return repo, nil
}

func cleanWorkingTree(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		relativePath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(source, relativePath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		return copyFile(path, targetPath)
	})
}

func copyFile(source, destination string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	return os.Chmod(destination, sourceInfo.Mode())
}
