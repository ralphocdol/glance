package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type SonarrInternal struct {
	Enable      bool              `yaml:"enable"`
	Endpoint    string            `yaml:"endpoint"`
	SkipSsl     bool              `yaml:"skipssl"`
	ApiKey      OptionalEnvString `yaml:"apikey"`
	Timezone    string            `yaml:"timezone"`
	ExternalUrl string            `yaml:"external-url"`
}

type RadarrInternal struct {
	Enable      bool              `yaml:"enable"`
	Endpoint    string            `yaml:"endpoint"`
	SkipSsl     bool              `yaml:"skipssl"`
	ApiKey      OptionalEnvString `yaml:"apikey"`
	Timezone    string            `yaml:"timezone"`
	ExternalUrl string            `yaml:"external-url"`
}

type ArrReleases struct {
	widgetBase    `yaml:",inline"`
	Releases      feed.ArrReleases `yaml:"-"`
	Sonarr        SonarrInternal   `yaml:"sonarr"`
	Radarr        RadarrInternal   `yaml:"radarr"`
	CollapseAfter int              `yaml:"collapse-after"`
	CacheDuration time.Duration    `yaml:"cache-duration"`
}

func convertToSonarrConfig(s SonarrInternal) feed.SonarrConfig {
	return feed.SonarrConfig{
		Enable:      s.Enable,
		Endpoint:    s.Endpoint,
		SkipSsl:     s.SkipSsl,
		ApiKey:      string(s.ApiKey),
		Timezone:    s.Timezone,
		ExternalUrl: s.ExternalUrl,
	}
}

func convertToRadarrConfig(r RadarrInternal) feed.RadarrConfig {
	return feed.RadarrConfig{
		Enable:      r.Enable,
		Endpoint:    r.Endpoint,
		SkipSsl:     r.SkipSsl,
		ApiKey:      string(r.ApiKey),
		Timezone:    r.Timezone,
		ExternalUrl: r.ExternalUrl,
	}
}

func (widget *ArrReleases) Initialize() error {
	widget.withTitle("Releasing Today")

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

func (widget *ArrReleases) Update(ctx context.Context) {
	sonarrConfig := convertToSonarrConfig(widget.Sonarr)
	radarrConfig := convertToRadarrConfig(widget.Radarr)
	releases, err := feed.FetchReleasesFromArrStack(sonarrConfig, radarrConfig)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Releases = releases
}

func (widget *ArrReleases) Render() template.HTML {
	return widget.render(widget, assets.ArrReleasesTemplate)
}
