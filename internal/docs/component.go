package docs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/Jeffail/benthos/v3/lib/util/config"
	"github.com/Jeffail/gabs/v2"
	"gopkg.in/yaml.v3"
)

// AnnotatedExample is an isolated example for a component.
type AnnotatedExample struct {
	// A title for the example.
	Title string

	// Summary of the example.
	Summary string

	// A config snippet to show.
	Config string
}

// Status of a component.
type Status string

// Component statuses.
var (
	StatusStable       Status = "stable"
	StatusBeta         Status = "beta"
	StatusExperimental Status = "experimental"
	StatusDeprecated   Status = "deprecated"
)

// Type of a component.
type Type string

// Component types.
var (
	TypeBuffer    Type = "buffer"
	TypeCache     Type = "cache"
	TypeInput     Type = "input"
	TypeMetrics   Type = "metrics"
	TypeOutput    Type = "output"
	TypeProcessor Type = "processor"
	TypeRateLimit Type = "rate_limit"
	TypeTracer    Type = "tracer"
)

// Types returns a slice containing all component types.
func Types() []Type {
	return []Type{
		TypeBuffer,
		TypeCache,
		TypeInput,
		TypeMetrics,
		TypeOutput,
		TypeProcessor,
		TypeRateLimit,
		TypeTracer,
	}
}

// ComponentSpec describes a Benthos component.
type ComponentSpec struct {
	// Name of the component
	Name string

	// Type of the component (input, output, etc)
	Type Type

	// The status of the component.
	Status Status

	// Plugin is true for all plugin components.
	Plugin bool

	// Summary of the component (in markdown, must be short).
	Summary string

	// Description of the component (in markdown).
	Description string

	// Categories that describe the purpose of the component.
	Categories []string

	// Footnotes of the component (in markdown).
	Footnotes string

	// Examples demonstrating use cases for the component.
	Examples []AnnotatedExample

	// A summary of each field in the component configuration.
	Config FieldSpec

	// Version is the Benthos version this component was introduced.
	Version string
}

type fieldContext struct {
	Name             string
	Type             string
	Description      string
	Default          string
	Advanced         bool
	Deprecated       bool
	Interpolated     bool
	Examples         []string
	AnnotatedOptions [][2]string
	Options          []string
	Version          string
}

type componentContext struct {
	Name               string
	Type               string
	FrontMatterSummary string
	Summary            string
	Description        string
	Categories         string
	Examples           []AnnotatedExample
	Fields             []fieldContext
	Footnotes          string
	CommonConfig       string
	AdvancedConfig     string
	Status             string
	Version            string
}

var componentTemplate = `{{define "field_docs" -}}
## Fields

{{range $i, $field := .Fields -}}
### ` + "`{{$field.Name}}`" + `

{{$field.Description}}
{{if $field.Interpolated -}}
This field supports [interpolation functions](/docs/configuration/interpolation#bloblang-queries).
{{end}}

Type: ` + "`{{$field.Type}}`" + `  
{{if gt (len $field.Default) 0}}Default: ` + "`{{$field.Default}}`" + `  
{{end -}}
{{if gt (len $field.Version) 0}}Requires version {{$field.Version}} or newer  
{{end -}}
{{if gt (len $field.AnnotatedOptions) 0}}
| Option | Summary |
|---|---|
{{range $j, $option := $field.AnnotatedOptions -}}` + "| `" + `{{index $option 0}}` + "` |" + ` {{index $option 1}} |
{{end}}
{{else if gt (len $field.Options) 0}}Options: {{range $j, $option := $field.Options -}}
{{if ne $j 0}}, {{end}}` + "`" + `{{$option}}` + "`" + `{{end}}.
{{end}}
{{if gt (len $field.Examples) 0 -}}
` + "```yaml" + `
# Examples

{{range $j, $example := $field.Examples -}}
{{if ne $j 0}}
{{end}}{{$example}}{{end -}}
` + "```" + `

{{end -}}
{{end -}}
{{end -}}

---
title: {{.Name}}
type: {{.Type}}
status: {{.Status}}
{{if gt (len .FrontMatterSummary) 0 -}}
description: "{{.FrontMatterSummary}}"
{{end -}}
{{if gt (len .Categories) 0 -}}
categories: {{.Categories}}
{{end -}}
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the contents of:
     lib/{{.Type}}/{{.Name}}.go
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

{{if eq .Status "beta" -}}
:::caution BETA
This component is mostly stable but breaking changes could still be made outside of major version releases if a fundamental problem with the component is found.
:::
{{end -}}
{{if eq .Status "experimental" -}}
:::caution EXPERIMENTAL
This component is experimental and therefore subject to change or removal outside of major version releases.
:::
{{end -}}
{{if eq .Status "deprecated" -}}
:::warning DEPRECATED
This component is deprecated and will be removed in the next major version release. Please consider moving onto [alternative components](#alternatives).
:::
{{end -}}

{{if gt (len .Summary) 0 -}}
{{.Summary}}
{{end -}}{{if gt (len .Version) 0}}
Introduced in version {{.Version}}.
{{end}}
{{if eq .CommonConfig .AdvancedConfig -}}
` + "```yaml" + `
# Config fields, showing default values
{{.CommonConfig -}}
` + "```" + `
{{else}}
<Tabs defaultValue="common" values={{"{"}}[
  { label: 'Common', value: 'common', },
  { label: 'Advanced', value: 'advanced', },
]{{"}"}}>

<TabItem value="common">

` + "```yaml" + `
# Common config fields, showing default values
{{.CommonConfig -}}
` + "```" + `

</TabItem>
<TabItem value="advanced">

` + "```yaml" + `
# All config fields, showing default values
{{.AdvancedConfig -}}
` + "```" + `

</TabItem>
</Tabs>
{{end -}}
{{if gt (len .Description) 0}}
{{.Description}}
{{end}}
{{if and (le (len .Fields) 4) (gt (len .Fields) 0) -}}
{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Examples) 0 -}}
## Examples

<Tabs defaultValue="{{ (index .Examples 0).Title }}" values={{"{"}}[
{{range $i, $example := .Examples -}}
  { label: '{{$example.Title}}', value: '{{$example.Title}}', },
{{end -}}
]{{"}"}}>

{{range $i, $example := .Examples -}}
<TabItem value="{{$example.Title}}">

{{if gt (len $example.Summary) 0 -}}
{{$example.Summary}}
{{end}}
{{if gt (len $example.Config) 0 -}}
` + "```yaml" + `{{$example.Config}}` + "```" + `
{{end}}
</TabItem>
{{end -}}
</Tabs>

{{end -}}

{{if gt (len .Fields) 4 -}}
{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Footnotes) 0 -}}
{{.Footnotes}}
{{end}}
`

func createOrderedConfig(t Type, rawExample interface{}, filter FieldFilter) (*yaml.Node, error) {
	var newNode yaml.Node
	if err := newNode.Encode(rawExample); err != nil {
		return nil, err
	}

	if err := SanitiseNode(t, &newNode, SanitiseConfig{
		RemoveTypeField: true,
		Filter:          filter,
		ForExample:      true,
	}); err != nil {
		return nil, err
	}

	return &newNode, nil
}

func genExampleConfigs(t Type, nest bool, fullConfigExample interface{}) (commonConfigStr, advConfigStr string, err error) {
	var advConfig, commonConfig interface{}
	if advConfig, err = createOrderedConfig(t, fullConfigExample, func(f FieldSpec) bool {
		return !f.Deprecated
	}); err != nil {
		panic(err)
	}
	if commonConfig, err = createOrderedConfig(t, fullConfigExample, func(f FieldSpec) bool {
		return !f.Advanced && !f.Deprecated
	}); err != nil {
		panic(err)
	}

	if nest {
		advConfig = map[string]interface{}{string(t): advConfig}
		commonConfig = map[string]interface{}{string(t): commonConfig}
	}

	advancedConfigBytes, err := config.MarshalYAML(advConfig)
	if err != nil {
		panic(err)
	}
	commonConfigBytes, err := config.MarshalYAML(commonConfig)
	if err != nil {
		panic(err)
	}

	return string(commonConfigBytes), string(advancedConfigBytes), nil
}

// AsMarkdown renders the spec of a component, along with a full configuration
// example, into a markdown document.
func (c *ComponentSpec) AsMarkdown(nest bool, fullConfigExample interface{}) ([]byte, error) {
	if strings.Contains(c.Summary, "\n\n") {
		return nil, fmt.Errorf("%v component '%v' has a summary containing empty lines", c.Type, c.Name)
	}

	ctx := componentContext{
		Name:        c.Name,
		Type:        string(c.Type),
		Summary:     c.Summary,
		Description: c.Description,
		Examples:    c.Examples,
		Footnotes:   c.Footnotes,
		Status:      string(c.Status),
		Version:     c.Version,
	}
	if ctx.Status == "" {
		ctx.Status = string(StatusStable)
	}

	if len(c.Categories) > 0 {
		cats, _ := json.Marshal(c.Categories)
		ctx.Categories = string(cats)
	}

	var err error
	if ctx.CommonConfig, ctx.AdvancedConfig, err = genExampleConfigs(c.Type, nest, fullConfigExample); err != nil {
		return nil, err
	}

	if len(c.Description) > 0 && c.Description[0] == '\n' {
		ctx.Description = c.Description[1:]
	}
	if len(c.Footnotes) > 0 && c.Footnotes[0] == '\n' {
		ctx.Footnotes = c.Footnotes[1:]
	}

	flattenedFields := c.Config.FlattenChildrenForDocs()
	gConf := gabs.Wrap(fullConfigExample).S(c.Name)
	for _, v := range flattenedFields {
		var defaultValue *interface{}
		if v.Default != nil {
			defaultValue = v.Default
		} else if dv := gConf.Path(v.Name).Data(); dv != nil {
			defaultValue = &dv
		}

		var defaultValueStr string
		if len(v.Children) == 0 && defaultValue != nil {
			defaultValueStr = gabs.Wrap(defaultValue).String()
		}

		fieldType := v.Type
		isArray := v.IsArray
		if len(fieldType) == 0 {
			if len(v.Examples) > 0 {
				fieldType, isArray = getFieldTypeFromInterface(v.Examples[0])
			} else if defaultValue != nil {
				fieldType, isArray = getFieldTypeFromInterface(*defaultValue)
			} else {
				return nil, fmt.Errorf("field '%v' not found in config example and no type or default value was provided in the spec", v.Name)
			}
		}
		fieldTypeStr := string(fieldType)
		if isArray {
			fieldTypeStr = "array"
		}
		if v.IsMap {
			fieldTypeStr = "object"
		}

		fieldCtx := fieldContext{
			Name:             v.Name,
			Type:             fieldTypeStr,
			Description:      v.Description,
			Default:          defaultValueStr,
			Advanced:         v.Advanced,
			Examples:         v.ExamplesMarshalled,
			AnnotatedOptions: v.AnnotatedOptions,
			Options:          v.Options,
			Interpolated:     v.Interpolated,
			Version:          v.Version,
		}

		if fieldCtx.Description == "" {
			fieldCtx.Description = "Sorry! This field is missing documentation."
		}

		if fieldCtx.Description[0] == '\n' {
			fieldCtx.Description = fieldCtx.Description[1:]
		}

		ctx.Fields = append(ctx.Fields, fieldCtx)
	}

	var buf bytes.Buffer
	err = template.Must(template.New("component").Parse(componentTemplate)).Execute(&buf, ctx)

	return buf.Bytes(), err
}
