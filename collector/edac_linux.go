// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !noedac

package collector

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	edacSubsystem = "edac"
)

var (
	edacMemControllerRE = regexp.MustCompile(`.*devices/system/edac/mc/mc([0-9]*)`)
	edacMemDimmRE       = regexp.MustCompile(`.*devices/system/edac/mc/mc[0-9]*/dimm([0-9]*)`)
	edacMemChannelRE    = regexp.MustCompile(`.*devices/system/edac/mc/mc([0-9]*)/ch([0-9]*)`)
)

type edacCollector struct {
	ceCount        *prometheus.Desc
	ueCount        *prometheus.Desc
	channelCECount *prometheus.Desc
	channelUECount *prometheus.Desc
	dimmCECount    *prometheus.Desc
	dimmUECount    *prometheus.Desc
	dimmLabel      *prometheus.Desc
	logger         *slog.Logger
}

func init() {
	registerCollector("edac", defaultEnabled, NewEdacCollector)
}

func NewEdacCollector(logger *slog.Logger) (Collector, error) {
	return &edacCollector{
		ceCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "correctable_errors_total"),
			"Total correctable memory errors.",
			[]string{"controller"},
			nil,
		),
		ueCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "uncorrectable_errors_total"),
			"Total uncorrectable memory errors.",
			[]string{"controller"},
			nil,
		),
		channelCECount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "channel_correctable_errors_total"),
			"Total correctable memory errors for this channel.",
			[]string{"controller", "channel"},
			nil,
		),
		channelUECount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "channel_uncorrectable_errors_total"),
			"Total uncorrectable memory errors for this channel.",
			[]string{"controller", "channel"},
			nil,
		),
		dimmCECount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "dimm_correctable_errors_total"),
			"Total correctable memory errors for this dimm.",
			[]string{"controller", "dimm"},
			nil,
		),
		dimmUECount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "dimm_uncorrectable_errors_total"),
			"Total uncorrectable memory errors for this dimm.",
			[]string{"controller", "dimm"},
			nil,
		),
		dimmLabel: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, edacSubsystem, "dimm_label"),
			"Label of the dimm.",
			[]string{"controller", "dimm", "channel", "label"},
			nil,
		),
		logger: logger,
	}, nil
}

func (c *edacCollector) Update(ch chan<- prometheus.Metric) error {

	memControllers, err := filepath.Glob(sysFilePath("devices/system/edac/mc/mc[0-9]*"))
	if err != nil {
		return err
	}

	for _, controller := range memControllers {

		controllerMatch := edacMemControllerRE.FindStringSubmatch(controller)
		if controllerMatch == nil {
			return fmt.Errorf("controller string didn't match regexp: %s", controller)
		}

		controllerNumber := controllerMatch[1]

		value, err := readUintFromFile(filepath.Join(controller, "ce_count"))
		if err == nil {
			ch <- prometheus.MustNewConstMetric(
				c.ceCount,
				prometheus.CounterValue,
				float64(value),
				controllerNumber,
			)
		}

		value, err = readUintFromFile(filepath.Join(controller, "ue_count"))
		if err == nil {
			ch <- prometheus.MustNewConstMetric(
				c.ueCount,
				prometheus.CounterValue,
				float64(value),
				controllerNumber,
			)
		}

		channels, err := filepath.Glob(controller + "/ch[0-9]*")
		if err != nil {
			return err
		}

		for _, channelPath := range channels {

			channelMatch := edacMemChannelRE.FindStringSubmatch(channelPath)
			if channelMatch == nil {
				continue
			}

			channelNumber := channelMatch[2]

			value, err := readUintFromFile(filepath.Join(channelPath, "ce_count"))
			if err == nil {
				ch <- prometheus.MustNewConstMetric(
					c.channelCECount,
					prometheus.CounterValue,
					float64(value),
					controllerNumber,
					channelNumber,
				)
			}

			value, err = readUintFromFile(filepath.Join(channelPath, "ue_count"))
			if err == nil {
				ch <- prometheus.MustNewConstMetric(
					c.channelUECount,
					prometheus.CounterValue,
					float64(value),
					controllerNumber,
					channelNumber,
				)
			}
		}

		dimms, err := filepath.Glob(controller + "/dimm[0-9]*")
		if err != nil {
			return err
		}

		for _, dimm := range dimms {

			dimmMatch := edacMemDimmRE.FindStringSubmatch(dimm)
			if dimmMatch == nil || len(dimmMatch) < 2 {
				continue
			}

			dimmNumber := dimmMatch[1]

			value, err := readUintFromFile(filepath.Join(dimm, "dimm_ce_count"))
			if err == nil {
				ch <- prometheus.MustNewConstMetric(
					c.dimmCECount,
					prometheus.CounterValue,
					float64(value),
					controllerNumber,
					dimmNumber,
				)
			}

			value, err = readUintFromFile(filepath.Join(dimm, "dimm_ue_count"))
			if err == nil {
				ch <- prometheus.MustNewConstMetric(
					c.dimmUECount,
					prometheus.CounterValue,
					float64(value),
					controllerNumber,
					dimmNumber,
				)
			}

			labelBytes, err := os.ReadFile(filepath.Join(dimm, "dimm_label"))
			if err == nil {

				label := strings.TrimSpace(string(labelBytes))
				channel := "unknown"

				channelMatch := regexp.MustCompile(`Chan#([0-9]+)`).FindStringSubmatch(label)
				if channelMatch != nil {
					channel = channelMatch[1]
				}

				ch <- prometheus.MustNewConstMetric(
					c.dimmLabel,
					prometheus.GaugeValue,
					1,
					controllerNumber,
					dimmNumber,
					channel,
					label,
				)
			}
		}
	}

	return nil
}
