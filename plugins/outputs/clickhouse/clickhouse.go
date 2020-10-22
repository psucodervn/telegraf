package clickhouse

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mailru/dbr"
	"github.com/mailru/go-clickhouse"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/outputs"
)

type clickhouseMetric map[string]interface{}

func (cm *clickhouseMetric) GetColumns() []string {
	columns := make([]string, 0)

	for column := range *cm {
		columns = append(columns, column)
	}
	return columns
}
func (cm *clickhouseMetric) AddData(name string, value interface{}, overwrite bool) {
	if _, exists := (*cm)[name]; !overwrite && exists {
		return
	}

	(*cm)[name] = value
}

func newClickhouseMetric(metric telegraf.Metric) *clickhouseMetric {
	cm := &clickhouseMetric{}

	for name, value := range metric.Fields() {
		cm.AddData(name, value, true)
	}
	for name, value := range metric.Tags() {
		cm.AddData(name, value, true)
	}

	return cm
}

type clickhouseMetrics []*clickhouseMetric

func (cms *clickhouseMetrics) GetColumns() []string {
	if len(*cms) == 0 {
		return []string{}
	}

	randomMetric := (*cms)[0] // all previous metrics are same
	return randomMetric.GetColumns()
}
func (cms *clickhouseMetrics) AddMissingColumn(name string, value interface{}) {
	for _, metric := range *cms {
		metric.AddData(name, value, false)
	}
}
func (cms *clickhouseMetrics) AddMetric(metric telegraf.Metric) {
	newMetric := newClickhouseMetric(metric)

	if len(*cms) > 0 {
		randomMetric := (*cms)[0] // all previous metrics are same

		for name := range *newMetric {
			if _, exists := (*randomMetric)[name]; !exists {
				cms.AddMissingColumn(name, nil)
			}
		}

		for name := range *randomMetric {
			if _, exists := (*newMetric)[name]; !exists {
				newMetric.AddData(name, nil, false)
			}
		}
	}

	*cms = append(*cms, newMetric)
}
func (cms *clickhouseMetrics) GetRowsByColumns(columns []string, jsonFields []string) [][]interface{} {
	rows := make([][]interface{}, 0)

	isJsonField := make(map[string]bool)
	for _, val := range jsonFields {
		isJsonField[val] = true
	}

	for _, metric := range *cms {
		row := make([]interface{}, 0)
		for _, column := range columns {
			value := (*metric)[column]
			if isJsonField[column] {
				switch typed_value := value.(type) {
				case string:
					var newValue interface{}
					err := json.Unmarshal([]byte(typed_value), &newValue)
					if err != nil {
						log.Printf("E! [clickhouse.embed_parser] could not parse field %s: %v", column, err)
					} else {
						value = newValue
					}
				default:
				}
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}

	return rows
}

type ClickhouseClient struct {
	URL        string
	Database   string
	SQLs       []string `toml:"create_sql"`
	JSONFields []string `toml:"json_fields"`

	timeout    time.Duration
	connection *dbr.Connection
}

func (c *ClickhouseClient) Connect() error {
	connection, err := dbr.Open("clickhouse", c.URL+"/"+c.Database, nil)
	if err != nil {
		log.Printf("[E] connect clickhouse failed: %v\n", err)
		return err
	}

	c.connection = connection

	err = c.connection.Ping()
	if err != nil {
		log.Printf("[E] ping clickhouse failed: %v\n", err)
		return err
	}

	for _, create_sql := range c.SQLs {
		_, err := c.connection.Exec(create_sql)
		if err != nil {
			log.Printf("[E] exec create_sql clickhouse failed: %v\n", err)
			return err
		}
	}

	return nil
}

func (c *ClickhouseClient) Close() error {
	return nil
}

func (c *ClickhouseClient) Description() string {
	return "Configuration for clickhouse server to send metrics to"
}

func (c *ClickhouseClient) SampleConfig() string {
	return `
# URL to connect
url = "http://localhost:8123"
# Database to use
database = "default"
# SQLs to create tables
create_sql = ["CREATE TABLE IF NOT EXISTS blablabla""]
`
}

func (c *ClickhouseClient) Write(metrics []telegraf.Metric) (err error) {
	err = nil
	inserts := make(map[string]*clickhouseMetrics)

	for _, metric := range metrics {
		table := c.Database + "." + metric.Name()

		if _, exists := inserts[table]; !exists {
			inserts[table] = &clickhouseMetrics{}
		}

		inserts[table].AddMetric(metric)
	}

	for table, insert := range inserts {
		if len(*insert) == 0 {
			continue
		}

		columns := insert.GetColumns()
		rows := insert.GetRowsByColumns(columns, c.JSONFields)

		fmt.Printf("[%v] Writing CH [%#v]: %#v : %#v\n", time.Now().Format(time.RFC850), table, len(rows), len(columns))

		colCount := len(columns)
		rowCount := len(rows)
		args := make([]interface{}, colCount*rowCount)
		argi := 0

		for _, row := range rows {
			for _, val := range row {
				switch typed_val := val.(type) {
				case []interface{}:
					args[argi] = clickhouse.Array(typed_val)
				default:
					args[argi] = typed_val
				}
				argi++
			}
		}

		binds := strings.Repeat("?,", colCount)
		binds = "(" + binds[:len(binds)-1] + "),"
		batch := strings.Repeat(binds, rowCount)
		batch = batch[:len(batch)-1]

		stmtCode := "INSERT INTO " + table + "(" + strings.Join(columns, ",") + ") VALUES " + batch

		stmt, err := c.connection.Prepare(stmtCode)

		if err != nil {
			log.Printf("E! [clickhouse.sql] error preparing sql stmt: %v", err)
			continue
		}

		_, err = stmt.Exec(args...)
		if err != nil {
			stmtCodeToDisplay := stmtCode
			if len(stmtCodeToDisplay) > 300 {
				stmtCodeToDisplay = stmtCodeToDisplay[:300]
			}
			log.Printf("[%v] E! [clickhouse.sql.err] error running sql stmt: %v\n query=%#v\n args=%#v .", time.Now().Format(time.RFC850), err, stmtCodeToDisplay, len(args))
			//log.Printf("E! [clickhouse.sql] error running sql stmt: %v, query=%v, args=%#v |", err, stmtCode, args)
			continue
		}
	}

	return err
}

func newClickhouse() *ClickhouseClient {
	var client = &ClickhouseClient{
		Database: "default",
		timeout:  time.Minute,
	}
	return client
}

func init() {
	outputs.Add("clickhouse", func() telegraf.Output {
		return newClickhouse()
	})
}
