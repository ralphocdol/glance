package feed

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type SonarrConfig struct {
	InternalUrl      string `yaml:"internal-url"`
	SkipSsl          bool   `yaml:"skipssl"`
	ApiKey           string `yaml:"apikey"`
	ExternalUrl      string `yaml:"external-url"`
	Timezone         string `yaml:"timezone"`
	FromPreviousDays int    `yaml:"from-previous-days"`
	Tags             string `yaml:"tags"`
}

type SonarrRelease struct {
	Title         string
	EpisodeTitle  string
	ImageCoverUrl string
	AirDateUtc    string
	SeasonNumber  *string
	EpisodeNumber *string
	Grabbed       bool
	Url           string
}

type SonarrReleases []SonarrRelease

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
	AirDateUtc   string `json:"airDateUtc"`
	EpisodeTitle string `json:"title"`
}

func HandleSonarrReleaseDatesTimezone(airDate time.Time, CustomTimezone string) (string, error) {
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

func FetchReleasesFromSonarr(Sonarr SonarrConfig) (SonarrReleases, error) {
	if Sonarr.InternalUrl == "" {
		return nil, fmt.Errorf("missing sonarr internal-url config")
	}

	if Sonarr.ApiKey == "" {
		return nil, fmt.Errorf("missing sonarr-apikey config")
	}

	if Sonarr.FromPreviousDays < 0 || Sonarr.FromPreviousDays > 6 {
		Sonarr.FromPreviousDays = 0
	}

	var appendPreviousDays string
	if Sonarr.FromPreviousDays != 0 {
		timeFormat := "2006-01-02T15:04:05Z"
		currentDate, err := time.Parse(timeFormat, time.Now().Format("2006-01-02T")+"00:00:00Z")
		if err == nil {
			previousDays := currentDate.AddDate(0, 0, -Sonarr.FromPreviousDays)
			appendPreviousDays = fmt.Sprintf("&start=%s", previousDays.Format(timeFormat))
		}
	}

	var appendTags string
	if Sonarr.Tags != "" {
		appendTags = fmt.Sprintf("&tags=%s", Sonarr.Tags)
	}

	appendParameters := appendPreviousDays + appendTags

	url := fmt.Sprintf("%s/api/v3/calendar?includeSeries=true%s", strings.TrimSuffix(Sonarr.InternalUrl, "/"), appendParameters)
	httpRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set("X-Api-Key", Sonarr.ApiKey)

	var clientType *http.Client
	if Sonarr.SkipSsl {
		clientType = defaultInsecureClient
	} else {
		clientType = defaultClient
	}

	response, err := decodeJsonFromRequest[[]SonarrReleaseResponse](clientType, httpRequest)
	if err != nil {
		return nil, err
	}

	var releases SonarrReleases
	for _, release := range response {
		var imageCover string
		for _, image := range release.Series.Images {
			if image.CoverType == "poster" {
				imageCover = image.RemoteUrl
				break
			}
		}

		airDate, err := time.Parse(time.RFC3339, release.AirDateUtc)
		if err != nil {
			return nil, fmt.Errorf("failed to parse air date: %v", err)
		}

		formattedDate, err := HandleSonarrReleaseDatesTimezone(airDate, Sonarr.Timezone)
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
			url = Sonarr.InternalUrl
		}
		linkUrl := fmt.Sprintf("%s/series/%s", strings.TrimSuffix(url, "/"), release.Series.TitleSlug)

		releases = append(releases, SonarrRelease{
			Title:         release.Series.Title,
			EpisodeTitle:  release.EpisodeTitle,
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

func FetchReleasesFromSonarrStack(Sonarr SonarrConfig) (SonarrReleases, error) {
	result := SonarrReleases{}

	sonarrReleases, err := FetchReleasesFromSonarr(Sonarr)
	if err != nil {
		slog.Warn("failed to fetch release from sonarr", "error", err)
		return nil, err
	}

	result = append(result, sonarrReleases...)

	return result, nil
}
