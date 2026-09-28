// Command files uploads FILE to CHAT_ID, downloads it back and checks it is unchanged.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func main() {
	// ctx ends on Ctrl+C, which cancels the transfer.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Getenv("BOT_TOKEN"), os.Getenv("CHAT_ID"), os.Getenv("FILE"), log.New(os.Stdout, "", 0))
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token, chat, path string, out *log.Logger, opts ...teleiq.Option) error {
	if token == "" || chat == "" {
		return errors.New("set BOT_TOKEN and CHAT_ID; FILE optionally names the file to upload")
	}
	chatID, err := parseChatID(chat)
	if err != nil {
		return err
	}
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}

	// Transfers have no default timeout, so give them one.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// want is the SHA-256 digest that the downloaded copy must match.
	content := []byte("Sent and downloaded back by the teleiq files example.\n")
	name, upload, want := "teleiq-example.txt", models.FileFromBytes("teleiq-example.txt", content), sha256.Sum256(content)
	if path != "" {
		name, upload = filepath.Base(path), models.FileFromPath(path)
		if want, err = fileDigest(path); err != nil {
			return err
		}
	}

	msg, err := client.SendDocument(ctx, teleiq.SendDocumentParams{ChatID: chatID, Document: upload})
	if err != nil {
		return err
	}
	if msg.Document == nil {
		return errors.New("the sent message has no document")
	}
	out.Printf("Uploaded %s", name)

	// info holds the path to download the file from.
	info, err := client.GetFile(ctx, teleiq.GetFileParams{FileID: msg.Document.FileID})
	if err != nil {
		return err
	}
	if info.FilePath == nil {
		return errors.New("the file cannot be downloaded")
	}
	dst, err := os.CreateTemp("", "teleiq-*-"+name) // the temporary file for the download
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(dst.Name()) }()
	// Writes to dst and sum at once, hashing on the way.
	sum := sha256.New()
	err = client.Download(ctx, *info.FilePath, io.MultiWriter(dst, sum))
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if [sha256.Size]byte(sum.Sum(nil)) != want {
		return fmt.Errorf("the downloaded file differs from %s", name)
	}
	out.Printf("Downloaded it back to a temporary file; the content matches")
	return nil
}

// fileDigest returns the SHA-256 digest of the file at path.
func fileDigest(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	digest := sha256.New()
	_, err = io.Copy(digest, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return [sha256.Size]byte(digest.Sum(nil)), err
}

// parseChatID reads CHAT_ID: a numeric ID or an @username.
func parseChatID(s string) (models.ChatID, error) {
	if strings.HasPrefix(s, "@") {
		return models.Username(s), nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return models.ChatID{}, fmt.Errorf("CHAT_ID must be a numeric chat ID or an @username, not %q", s)
	}
	return models.ID(id), nil
}
