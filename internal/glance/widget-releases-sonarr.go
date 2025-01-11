package glance

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var releasesSonarrWidgetTemplate = mustParseTemplate("releases-sonarr.html", "widget-base.html")

type releasesSonarrWidget struct {
	widgetBase    `yaml:",inline"`
	Releases      releasesSonarr `yaml:"-"`
	Config        sonarrConfig   `yaml:"config"`
	CollapseAfter int            `yaml:"collapse-after"`
	CacheDuration time.Duration  `yaml:"cache-duration"`
}

type sonarrConfig struct {
	InternalUrl       string `yaml:"internal-url"`
	SkipSsl           bool   `yaml:"skipssl"`
	ApiKey            string `yaml:"apikey"`
	ExternalUrl       string `yaml:"external-url"`
	Timezone          string `yaml:"timezone"`
	DayOffset         int    `yaml:"day-offset"`
	FromPreviousDays  int    `yaml:"from-previous-days"`
	Tags              string `yaml:"tags"`
	InternalThumbnail bool   `yaml:"internal-insecure-thumbnail"`
}

type releaseSonarr struct {
	Title         string
	EpisodeTitle  string
	ImageCoverUrl string
	AirDateUtc    string
	SeasonNumber  *string
	EpisodeNumber *string
	Grabbed       bool
	Url           string
}

type releasesSonarr []releaseSonarr

type sonarrReleaseResponse struct {
	HasFile       bool `json:"hasFile"`
	SeriesId      int  `json:"seriesId"`
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

func (widget *releasesSonarrWidget) initialize() error {
	widget.withTitle("Sonarr: Releasing Today")

	// Set cache duration
	if widget.CacheDuration == 0 {
		widget.CacheDuration = time.Minute * 5
	}
	widget.withCacheDuration(widget.CacheDuration)

	// Set collapse after default value
	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	return nil
}

func (widget *releasesSonarrWidget) update(ctx context.Context) {
	releases, err := fetchReleasesFromSonarrStack(widget.Config)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Releases = releases
}

func (widget *releasesSonarrWidget) Render() template.HTML {
	return widget.renderTemplate(widget, releasesSonarrWidgetTemplate)
}

func fetchReleasesFromSonarrStack(Sonarr sonarrConfig) (releasesSonarr, error) {
	result := releasesSonarr{}

	sonarrReleases, err := fetchReleasesFromSonarr(Sonarr)
	if err != nil {
		slog.Warn("failed to fetch release from sonarr", "error", err)
		return nil, err
	}

	result = append(result, sonarrReleases...)

	return result, nil
}

func fetchReleasesFromSonarr(Sonarr sonarrConfig) (releasesSonarr, error) {
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
	httpRequest, err := querySonarrApi(Sonarr.InternalUrl, "calendar", appendParameters, Sonarr.ApiKey)
	if err != nil {
		return nil, err
	}

	var clientType = sonarrSkipSsl(Sonarr.SkipSsl)

	response, err := decodeJsonFromRequest[[]sonarrReleaseResponse](clientType, httpRequest)
	if err != nil {
		return nil, err
	}

	var releases releasesSonarr
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

		if Sonarr.InternalThumbnail {
			// will show API key, insecure
			seriesId := fmt.Sprintf("mediacover/%s/poster-500.jpg", strconv.Itoa(release.SeriesId))
			imageCover = buildSonarrQuery(Sonarr.ExternalUrl, seriesId, "", Sonarr.ApiKey)
		} else {
			for _, image := range release.Series.Images {
				if image.CoverType == "poster" {
					imageCover = image.RemoteUrl
					break
				}
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

		releases = append(releases, releaseSonarr{
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

func handleSonarrReleaseDatesTimezone(airDate time.Time, CustomTimezone string) (string, error) {
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

func querySonarrApi(urlPath string, apiPath string, params string, apiKey string) (*http.Request, error) {
	url := buildSonarrQuery(urlPath, apiPath, params, "")
	httpRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set("X-Api-Key", apiKey)

	return httpRequest, err
}

func buildSonarrQuery(urlPath string, apiPath string, params string, apiKey string) string {
	newParam := ""
	appendApiKey := "apikey=" + apiKey
	if params != "" {
		newParam = "?" + params
		if apiKey != "" {
			newParam += "&" + appendApiKey
		}
	} else if apiKey != "" {
		newParam = "?" + appendApiKey
	}
	url := fmt.Sprintf("%s/api/v3/%s%s", strings.TrimSuffix(urlPath, "/"), apiPath, newParam)

	return url
}

func sonarrSkipSsl(skipSsl bool) *http.Client {
	var clientType *http.Client
	if skipSsl {
		clientType = defaultInsecureHTTPClient
	} else {
		clientType = defaultHTTPClient
	}

	return clientType
}
