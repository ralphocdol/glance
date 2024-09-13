package feed

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SonarrConfig struct {
	InternalUrl      string `yaml:"internal-url"`
	SkipSsl          bool   `yaml:"skipssl"`
	ApiKey           string `yaml:"apikey"`
	ExternalUrl      string `yaml:"external-url"`
	Timezone         string `yaml:"timezone"`
	DayOffset        int    `yaml:"day-offset"`
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

	timeSetLocal := time.Now()
	timeSetUTC := timeSetLocal.UTC()
	if Sonarr.DayOffset != 0 {
		timeSetLocal = timeSetLocal.AddDate(0, 0, Sonarr.DayOffset)
		timeSetUTC = timeSetUTC.AddDate(0, 0, Sonarr.DayOffset)
	}
	startDateUTC := getStartOfDay(timeSetUTC, time.UTC)
	endDateUTC := getEndOfDay(timeSetUTC, time.UTC)

	timeLocal := time.Local

	var startDateLocal, endDateLocal time.Time
	if Sonarr.Timezone != "" {
		loc, err := time.LoadLocation(Sonarr.Timezone)
		if err != nil {
			return nil, err
		}
		timeLocal = loc
	}
	startDateLocal = getStartOfDay(timeSetLocal, timeLocal)
	endDateLocal = getEndOfDay(timeSetLocal, timeLocal)

	if Sonarr.FromPreviousDays != 0 {
		startDateUTC = startDateUTC.AddDate(0, 0, -Sonarr.FromPreviousDays)
		startDateLocal = startDateLocal.AddDate(0, 0, -Sonarr.FromPreviousDays)
	}

	var appendTags string
	if Sonarr.Tags != "" {
		appendTags = fmt.Sprintf("&tags=%s", Sonarr.Tags)
	}

	// Query ±1 date range
	dateRangeFilter := fmt.Sprintf("&start=%s&end=%s",
		url.QueryEscape(startDateUTC.AddDate(0, 0, -1).Format(time.RFC3339)),
		url.QueryEscape(endDateUTC.AddDate(0, 0, 1).Format(time.RFC3339)),
	)

	appendParameters := appendTags + dateRangeFilter
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
		airDate, err := time.Parse(time.RFC3339, release.AirDateUtc)
		if err != nil {
			return nil, err
		}

		airDateLocal := airDate.In(timeLocal)
		if airDateLocal.Before(startDateLocal) || airDateLocal.After(endDateLocal) {
			continue
		}

		var imageCover string
		for _, image := range release.Series.Images {
			if image.CoverType == "poster" {
				imageCover = image.RemoteUrl
				break
			}
		}

		formattedDate := airDateLocal.Format("01-02 15:04")

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
