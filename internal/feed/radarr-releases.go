package feed

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type RadarrConfig struct {
	InternalUrl string `yaml:"internal-url"`
	SkipSsl     bool   `yaml:"skipssl"`
	ApiKey      string `yaml:"apikey"`
	ExternalUrl string `yaml:"external-url"`
	Timezone    string `yaml:"timezone"`
}

type RadarrRelease struct {
	Title         string
	Overview      string
	ImageCoverUrl string
	AirDateUtc    string
	SeasonNumber  *string
	EpisodeNumber *string
	Grabbed       bool
	Url           string
}

type RadarrReleases []RadarrRelease

type RadarrReleaseResponse struct {
	HasFile bool   `json:"hasFile"`
	Title   string `json:"title"`
	Images  []struct {
		CoverType string `json:"coverType"`
		RemoteUrl string `json:"remoteUrl"`
	} `json:"images"`
	AirDateUtc          string `json:"airDateUtc"`
	InCinemasDate       string `json:"inCinemas"`
	PhysicalReleaseDate string `json:"physicalRelease"`
	DigitalReleaseDate  string `json:"digitalRelease"`
	ReleaseDate         string `json:"releaseDate"`
	TitleSlug           string `json:"titleSlug"`
	Overview            string `json:"overview"`
}

func HandleRadarrReleaseDatesTimezone(airDate time.Time, CustomTimezone string) (string, error) {
	var formattedDate string
	if CustomTimezone != "" {
		location, err := time.LoadLocation(CustomTimezone)
		if err != nil {
			return "", fmt.Errorf("failed to load location: %v", err)
		}

		// Convert the parsed time to the new time zone
		airDateInLocation := airDate.In(location)

		// Format the date as MM-DD HH:SS in the new time zone
		formattedDate = airDateInLocation.Format("01-02 15:04")
	} else {
		// Format the date as MM-DD
		formattedDate = airDate.Format("01-02 15:04")
	}

	return formattedDate, nil
}

func FetchReleasesFromRadarr(Radarr RadarrConfig) (RadarrReleases, error) {
	if Radarr.InternalUrl == "" {
		return nil, fmt.Errorf("missing radarr internal-url config")
	}

	if Radarr.ApiKey == "" {
		return nil, fmt.Errorf("missing radarr-apikey config")
	}

	url := fmt.Sprintf("%s/api/v3/calendar", strings.TrimSuffix(Radarr.InternalUrl, "/"))
	httpRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set("X-Api-Key", Radarr.ApiKey)

	var clientType *http.Client
	if Radarr.SkipSsl {
		clientType = defaultInsecureClient
	} else {
		clientType = defaultClient
	}
	response, err := decodeJsonFromRequest[[]RadarrReleaseResponse](clientType, httpRequest)

	var releases RadarrReleases
	for _, release := range response {
		var imageCover string
		for _, image := range release.Images {
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

		// Choose the appropriate release date from Radarr's response
		var releaseDate string
		var formattedDate string
		if release.ReleaseDate != "" {
			releaseDate = release.ReleaseDate
			formattedDate = ""
		} else if release.InCinemasDate != "" {
			releaseDate = release.InCinemasDate
			formattedDate = "Cinemas: "
		} else if release.PhysicalReleaseDate != "" {
			releaseDate = release.PhysicalReleaseDate
			formattedDate = "Physical: "
		} else if release.DigitalReleaseDate != "" {
			releaseDate = release.DigitalReleaseDate
			formattedDate = "Digital: "
		}

		airDate, err := time.Parse(time.RFC3339, releaseDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse release date: %v", err)
		}

		timezoneDate, err := HandleRadarrReleaseDatesTimezone(airDate, Radarr.Timezone)
		if err != nil {
			return nil, fmt.Errorf("failed to parse air timezone: %v", err)
		}

		formattedDate = formattedDate + timezoneDate

		var url string
		if Radarr.ExternalUrl != "" {
			url = Radarr.ExternalUrl
		} else {
			url = Radarr.InternalUrl
		}
		linkUrl := fmt.Sprintf("%s/movie/%s", strings.TrimSuffix(url, "/"), release.TitleSlug)

		releases = append(releases, RadarrRelease{
			Title:         release.Title,
			Overview:      overview,
			ImageCoverUrl: imageCover,
			AirDateUtc:    formattedDate,
			Grabbed:       release.HasFile,
			Url:           linkUrl,
		})
	}

	return releases, nil
}

func FetchReleasesFromRadarrStack(Radarr RadarrConfig) (RadarrReleases, error) {
	result := RadarrReleases{}

	radarrReleases, err := FetchReleasesFromRadarr(Radarr)
	if err != nil {
		slog.Warn("failed to fetch release from radarr", "error", err)
		return nil, err
	}

	result = append(result, radarrReleases...)

	return result, nil
}
