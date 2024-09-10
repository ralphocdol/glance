package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type SonarrInternal struct {
	InternalUrl string            `yaml:"internal-url"`
	SkipSsl     bool              `yaml:"skipssl"`
	ApiKey      OptionalEnvString `yaml:"apikey"`
	Timezone    string            `yaml:"timezone"`
	ExternalUrl string            `yaml:"external-url"`
}

type SonarrReleases struct {
	widgetBase    `yaml:",inline"`
	Releases      feed.SonarrReleases `yaml:"-"`
	Sonarr        SonarrInternal      `yaml:"sonarr"`
	CollapseAfter int                 `yaml:"collapse-after"`
	CacheDuration time.Duration       `yaml:"cache-duration"`
}

func convertToSonarrConfig(s SonarrInternal) feed.SonarrConfig {
	return feed.SonarrConfig{
		InternalUrl: s.InternalUrl,
		SkipSsl:     s.SkipSsl,
		ApiKey:      string(s.ApiKey),
		Timezone:    s.Timezone,
		ExternalUrl: s.ExternalUrl,
	}
}

func (widget *SonarrReleases) Initialize() error {
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

func (widget *SonarrReleases) Update(ctx context.Context) {
	sonarrConfig := convertToSonarrConfig(widget.Sonarr)
	releases, err := feed.FetchReleasesFromSonarrStack(sonarrConfig)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Releases = releases
}

func (widget *SonarrReleases) Render() template.HTML {
	return widget.render(widget, assets.SonarrReleasesTemplate)
}
