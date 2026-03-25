package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	appnotify "pano_chart/backend/application/notifications"
	infranotify "pano_chart/backend/infrastructure/notifications"
	infrasocial "pano_chart/backend/infrastructure/social"
)

func main() {
	file := flag.String("file", "", "path to a markdown news file")
	msg := flag.String("msg", "", "direct notification message")
	title := flag.String("title", "Market News", "notification title")
	flag.Parse()

	if *file == "" && *msg == "" {
		fmt.Fprintln(os.Stderr, "usage: notify_news -msg 'text' | -file news.md [-title 'Title']")
		os.Exit(1)
	}

	var body string
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			log.Fatalf("reading file: %v", err)
		}
		body = extractExcerpt(string(data))
	} else {
		body = *msg
	}

	// --- Resolve environment ---
	deviceDBPath := os.Getenv("DEVICE_DB_PATH")
	if deviceDBPath == "" {
		deviceDBPath = "./device_tokens.sqlite"
	}
	fcmCredsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if fcmCredsPath == "" {
		log.Fatal("GOOGLE_APPLICATION_CREDENTIALS not set")
	}
	fcmProjectID := os.Getenv("FCM_PROJECT_ID")
	if fcmProjectID == "" {
		log.Fatal("FCM_PROJECT_ID not set")
	}

	// --- Build components ---
	deviceStore, err := infrasocial.NewSQLiteDeviceStore(deviceDBPath)
	if err != nil {
		log.Fatalf("device store: %v", err)
	}
	defer func() { _ = deviceStore.Close() }()

	fcm, err := infrasocial.NewFCMNotifier(fcmCredsPath, fcmProjectID)
	if err != nil {
		log.Fatalf("FCM notifier: %v", err)
	}

	sender := infranotify.NewBroadcastSender(deviceStore, fcm)
	engine := appnotify.NewEngine(sender, appnotify.DefaultEngineConfig())

	n := appnotify.Notification{
		Type:  appnotify.TypeNews,
		Title: *title,
		Body:  body,
		Key:   "news_" + hashKey(body),
	}

	if err := engine.Send(context.Background(), n); err != nil {
		log.Fatalf("send failed: %v", err)
	}
	fmt.Println("Sent")
}

// extractExcerpt returns the first few non-empty lines from markdown.
func extractExcerpt(md string) string {
	var lines []string
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		lines = append(lines, l)
		if len(lines) >= 3 {
			break
		}
	}
	return strings.Join(lines, " ")
}

// hashKey produces a short deterministic key from text to avoid giant dedup keys.
func hashKey(s string) string {
	if len(s) <= 64 {
		return s
	}
	return s[:64]
}
