package feed

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SonarrConfig struct {
	Enable      bool   `yaml:"enable"`
	Endpoint    string `yaml:"endpoint"`
	SkipSsl     bool   `yaml:"skipssl"`
	ApiKey      string `yaml:"apikey"`
	ExternalUrl string `yaml:"external-url"`
	Timezone    string `yaml:"timezone"`
}

type RadarrConfig struct {
	Enable      bool   `yaml:"enable"`
	Endpoint    string `yaml:"endpoint"`
	SkipSsl     bool   `yaml:"skipssl"`
	ApiKey      string `yaml:"apikey"`
	ExternalUrl string `yaml:"external-url"`
	Timezone    string `yaml:"timezone"`
}

type ArrRelease struct {
	Title         string
	Overview      string
	ImageCoverUrl string
	AirDateUtc    string
	SeasonNumber  *string
	EpisodeNumber *string
	Grabbed       bool
	Url           string
}

type ArrReleases []ArrRelease

type SonarrReleaseResponse struct {
	HasFile       bool `json:"hasFile"`
	SeasonNumber  int  `json:"seasonNumber"`
	EpisodeNumber int  `json:"episodeNumber"`
	Series        struct {
		Title  string `json:"title"`
		Images []struct {
			CoverType string `json:"coverType"`
			RemoteUrl string `json:"remoteUrl"`
		} `json:"images"`
		TitleSlug string `json:"titleSlug"`
	} `json:"series"`
	AirDateUtc string `json:"airDateUtc"`
	Overview   string `json:"overview"`
}

type RadarrReleaseResponse struct {
	HasFile bool   `json:"hasFile"`
	Title   string `json:"title"`
	Images  []struct {
		CoverType string `json:"coverType"`
		RemoteUrl string `json:"remoteUrl"`
	} `json:"images"`
	InCinemasDate       string `json:"inCinemas"`
	PhysicalReleaseDate string `json:"physicalRelease"`
	DigitalReleaseDate  string `json:"digitalRelease"`
}

func extractHostFromURL(apiEndpoint string) string {
	u, err := url.Parse(apiEndpoint)
	if err != nil {
		return "127.0.0.1"
	}
	return u.Host
}

func HandleReleaseDatesTimezone(airDate time.Time, CustomTimezone string) (string, error) {
	var formattedDate string
	if CustomTimezone != "" {
		location, err := time.LoadLocation(CustomTimezone)
		if err != nil {
			return "", fmt.Errorf("failed to load location: %v", err)
		}

		// Convert the parsed time to the new time zone
		airDateInLocation := airDate.In(location)

		// Format the date as YYYY-MM-DD HH:MM:SS in the new time zone
		formattedDate = airDateInLocation.Format("01/02 15:04")
	} else {
		// Format the date as YYYY-MM-DD HH:MM:SS
		formattedDate = airDate.Format("01/02 15:04")
	}

	return formattedDate, nil
}

func FetchApi(apiAddress string, apiEndpoint string, apiKey string, skipSsl bool) ([]byte, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipSsl},
	}
	client := &http.Client{
		Transport: transport,
	}
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(apiAddress, "/"), apiEndpoint)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Host", extractHostFromURL(apiEndpoint))
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}

func FetchReleasesFromSonarr(Sonarr SonarrConfig) (ArrReleases, error) {
	if Sonarr.Endpoint == "" {
		return nil, fmt.Errorf("missing sonarr-endpoint config")
	}

	if Sonarr.ApiKey == "" {
		return nil, fmt.Errorf("missing sonarr-apikey config")
	}

	body, err := FetchApi(Sonarr.Endpoint, "/api/v3/calendar?includeSeries=true", Sonarr.ApiKey, Sonarr.SkipSsl)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var sonarrReleases []SonarrReleaseResponse
	err = json.Unmarshal(body, &sonarrReleases)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	var releases ArrReleases
	for _, release := range sonarrReleases {
		var imageCover string
		for _, image := range release.Series.Images {
			if image.CoverType == "poster" {
				imageCover = image.RemoteUrl
				break
			}
		}

		// Determine overview to display
		var overview string
		if release.Overview == "" {
			overview = "TBA"
		} else {
			overview = release.Overview
		}

		airDate, err := time.Parse(time.RFC3339, release.AirDateUtc)
		if err != nil {
			return nil, fmt.Errorf("failed to parse air date: %v", err)
		}

		formattedDate, err := HandleReleaseDatesTimezone(airDate, Sonarr.Timezone)
		if err != nil {
			return nil, fmt.Errorf("failed to parse air timezone: %v", err)
		}

		// Format SeasonNumber and EpisodeNumber with at least two digits
		seasonNumber := fmt.Sprintf("%02d", release.SeasonNumber)
		episodeNumber := fmt.Sprintf("%02d", release.EpisodeNumber)

		var url string
		if Sonarr.ExternalUrl != "" {
			url = Sonarr.ExternalUrl
		} else {
			url = Sonarr.Endpoint
		}
		linkUrl := fmt.Sprintf("%s/series/%s", strings.TrimSuffix(url, "/"), release.Series.TitleSlug)

		releases = append(releases, ArrRelease{
			Title:         release.Series.Title,
			Overview:      overview,
			ImageCoverUrl: imageCover,
			AirDateUtc:    formattedDate,
			SeasonNumber:  &seasonNumber,
			EpisodeNumber: &episodeNumber,
			Grabbed:       release.HasFile,
			Url:           linkUrl,
		})
	}

	return releases, nil
}

func FetchReleasesFromRadarr(Radarr RadarrConfig) (ArrReleases, error) {
	if Radarr.Endpoint == "" {
		return nil, fmt.Errorf("missing radarr-endpoint config")
	}

	if Radarr.ApiKey == "" {
		return nil, fmt.Errorf("missing radarr-apikey config")
	}

	body, err := FetchApi(Radarr.Endpoint, "/api/v3/calendar?includeSeries=true", Radarr.ApiKey, Radarr.SkipSsl)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var radarrReleases []RadarrReleaseResponse
	err = json.Unmarshal(body, &radarrReleases)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	var releases ArrReleases
	for _, release := range radarrReleases {
		var imageCover string
		for _, image := range release.Images {
			if image.CoverType == "poster" {
				imageCover = image.RemoteUrl
				break
			}
		}

		// Choose the appropriate release date from Radarr's response
		releaseDate := release.InCinemasDate
		formattedDate := "In Cinemas: "
		if release.PhysicalReleaseDate != "" {
			releaseDate = release.PhysicalReleaseDate
			formattedDate = "Physical Release: "
		} else if release.DigitalReleaseDate != "" {
			releaseDate = release.DigitalReleaseDate
			formattedDate = "Digital Release: "
		}

		airDate, err := time.Parse("2006-01-02", releaseDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse release date: %v", err)
		}

		timezoneDate, err := HandleReleaseDatesTimezone(airDate, Radarr.Timezone)
		if err != nil {
			return nil, fmt.Errorf("failed to parse air timezone: %v", err)
		}

		// Format the date as YYYY-MM-DD HH:MM:SS
		formattedDate = formattedDate + timezoneDate

		// var url string
		// if Radarr.ExternalUrl != "" {
		// 	url = Radarr.ExternalUrl
		// } else {
		// 	url = Radarr.Endpoint
		// }
		// linkUrl := fmt.Sprintf("%s/series/%s", strings.TrimSuffix(url, "/"), release.Movies.id) // is it id?

		releases = append(releases, ArrRelease{
			Title:         release.Title,
			ImageCoverUrl: imageCover,
			AirDateUtc:    formattedDate,
			Grabbed:       release.HasFile,
		})
	}

	return releases, nil
}

func FetchReleasesFromArrStack(Sonarr SonarrConfig, Radarr RadarrConfig) (ArrReleases, error) {
	result := ArrReleases{}

	// Call FetchReleasesFromSonarr and handle the result
	if Sonarr.Enable {
		sonarrReleases, err := FetchReleasesFromSonarr(Sonarr)
		if err != nil {
			slog.Warn("failed to fetch release from sonarr", "error", err)
			return nil, err
		}

		result = append(result, sonarrReleases...)
	}

	// Call FetchReleasesFromRadarr and handle the result
	if Radarr.Enable {
		radarrReleases, err := FetchReleasesFromRadarr(Radarr)
		if err != nil {
			slog.Warn("failed to fetch release from radarr", "error", err)
			return nil, err
		}

		result = append(result, radarrReleases...)
	}

	return result, nil
}
