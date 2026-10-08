// File path: elegba/internal/step/transform.go

package step

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"text/template"

	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/pipeline"
)

type TransformStep struct {
	config   config.Step
	template *template.Template
}

func NewTransformStep(stepConfig config.Step) (*TransformStep, error) {
	tmpl, err := template.New(stepConfig.ID).Funcs(templateFunctions()).Option("missingkey=error").Parse(stepConfig.Template)
	if err != nil {
		return nil, err
	}
	return &TransformStep{config: stepConfig, template: tmpl}, nil
}

func (ts *TransformStep) ID() string          { return ts.config.ID }
func (ts *TransformStep) DependsOn() []string { return ts.config.DependsOn }

func (ts *TransformStep) Execute(_ context.Context, data map[string]any) (any, error) {
	var output bytes.Buffer
	err := ts.template.Execute(&output, data)
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

var _ pipeline.Step = (*TransformStep)(nil)

func templateFunctions() template.FuncMap {
	return template.FuncMap{
		"toJSON": func(value any) (string, error) {
			data, err := json.Marshal(value)
			return string(data), err
		},
		"fromJSON": func(value string) (any, error) {
			var result any
			decoder := json.NewDecoder(bytes.NewBufferString(value))
			decoder.UseNumber()
			if err := decoder.Decode(&result); err != nil {
				return nil, err
			}
			return result, nil
		},
		"len": func(value any) (int, error) {
			if value == nil {
				return 0, nil
			}
			reflected := reflect.ValueOf(value)
			switch reflected.Kind() {
			case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
				return reflected.Len(), nil
			default:
				return 0, fmt.Errorf("len does not support %T", value)
			}
		},
		"default": func(fallback, value any) any {
			if value == nil || value == "" || value == false {
				return fallback
			}
			return value
		},
		"add": func(left, right any) (float64, error) {
			leftNumber, err := number(left)
			if err != nil {
				return 0, err
			}
			rightNumber, err := number(right)
			if err != nil {
				return 0, err
			}
			return leftNumber + rightNumber, nil
		},
		"sum": func(value any) (float64, error) {
			items := reflect.ValueOf(value)
			if items.Kind() != reflect.Array && items.Kind() != reflect.Slice {
				return 0, fmt.Errorf("sum expects an array or slice")
			}
			var total float64
			for i := 0; i < items.Len(); i++ {
				item, err := number(items.Index(i).Interface())
				if err != nil {
					return 0, err
				}
				total += item
			}
			return total, nil
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict requires key-value pairs")
			}
			result := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				result[key] = values[i+1]
			}
			return result, nil
		},
		"sha256": func(value string) string {
			digest := sha256.Sum256([]byte(value))
			return hex.EncodeToString(digest[:])
		},
	}
}

func number(value any) (float64, error) {
	switch value := value.(type) {
	case json.Number:
		return value.Float64()
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case float64:
		return value, nil
	case string:
		return strconv.ParseFloat(value, 64)
	default:
		return 0, fmt.Errorf("%T is not numeric", value)
	}
}
