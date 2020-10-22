package embed_parser

import (
	"encoding/json"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/parsers"
	"github.com/influxdata/telegraf/plugins/processors"
	"log"
)

const sampleConfig = `
fields=["field1","field2"]
`

type EmbedParser struct {
	parsers.Config
	Fields []string `toml:"fields"`
	Parser parsers.Parser
}

func (r *EmbedParser) SampleConfig() string {
	return sampleConfig
}

func (r *EmbedParser) Description() string {
	return "fields that pass through this filter."
}

func (r *EmbedParser) Apply(in ...telegraf.Metric) []telegraf.Metric {
	for _, point := range in {
		for _, field := range r.Fields {
			if data, ok := point.GetField(field); ok {
				switch value := data.(type) {
				case string:
					point.RemoveField(field)
					var newValue []interface{}
					err := json.Unmarshal([]byte(value), &newValue)
					if err != nil {
						log.Printf("E! [processors.embed_parser] could not parse field %s: %v", field, err)
					} else {
						point.AddField(field, newValue)
					}
				default:
					log.Printf("E! [processors.embed_parser] field '%s' not a string, skipping", field)
				}
			} else {
				log.Printf("W! [processors.embed_parser] field does not exists %s in %v", field, point)
			}
		}
	}

	return in
}

func init() {
	processors.Add("embed_parser", func() telegraf.Processor {
		return &EmbedParser{}
	})
}
