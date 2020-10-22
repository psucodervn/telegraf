package drop_fields

import (
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/processors"
)

const sampleConfig = `
fields=["field1","field2"]
tags=["tag1_key","tag2_key"]
`

type DropField struct {
	Fields []string `toml:"fields"`
	Tags   []string `toml:"tags"`
}

func (r *DropField) SampleConfig() string {
	return sampleConfig
}

func (r *DropField) Description() string {
	return "DropField fields that pass through this filter."
}

func (r *DropField) Apply(in ...telegraf.Metric) []telegraf.Metric {
	for _, point := range in {
		for _, field := range r.Fields {
			if _, ok := point.GetField(field); ok {
				point.RemoveField(field)
			}
		}
		for _, tag := range r.Tags {
			if _, ok := point.GetTag(tag); ok {
				point.RemoveTag(tag)
			}
		}
	}

	return in
}

func init() {
	processors.Add("dropfields", func() telegraf.Processor {
		return &DropField{}
	})
}
