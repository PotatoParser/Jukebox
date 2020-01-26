package main

import (
	"encoding/json"
	"os/exec"
)

type Song struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	WebURL    string `json:"webURL"`
}

func fetchSong(url string) (Song, error) {
	// Maybe make this function return a promise or something in the future
	var songData Info
	// JSON dump has no overhead, and we get more info that we need that might be useful
	cmd := exec.Command("youtube-dl", "--no-playlist", "-J", "-f bestaudio/bestvideo", url)
	stdOut, err := cmd.StdoutPipe()

	if err != nil {
		return Song{}, err
	}

	if err := cmd.Start(); err != nil {
		return Song{}, err
	}

	if err := json.NewDecoder(stdOut).Decode(&songData); err != nil {
		return Song{}, err
	}

	cmd.Wait()
	// Only using these fields atm
	return Song{
		Title:     songData.Title,
		URL:       songData.URL,
		Thumbnail: songData.Thumbnail,
		WebURL:    url,
	}, nil
}