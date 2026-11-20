package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func schemaType(ctx context.Context, r resource.Resource) tftypes.Type {
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	return resp.Schema.Type().TerraformType(ctx)
}

func pruneToSchema(raw json.RawMessage, typ tftypes.Type) (json.RawMessage, error) {
	switch t := typ.(type) {
	case tftypes.Object:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		if fields == nil {
			return raw, nil
		}
		for name, value := range fields {
			attr, ok := t.AttributeTypes[name]
			if !ok {
				delete(fields, name)
				continue
			}
			pruned, err := pruneToSchema(value, attr)
			if err != nil {
				return nil, err
			}
			fields[name] = pruned
		}
		return json.Marshal(fields)
	case tftypes.List:
		return pruneEach(raw, t.ElementType)
	case tftypes.Set:
		return pruneEach(raw, t.ElementType)
	case tftypes.Map:
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		if entries == nil {
			return raw, nil
		}
		for k, v := range entries {
			pruned, err := pruneToSchema(v, t.ElementType)
			if err != nil {
				return nil, err
			}
			entries[k] = pruned
		}
		return json.Marshal(entries)
	default:
		return raw, nil
	}
}

func pruneEach(raw json.RawMessage, elem tftypes.Type) (json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	if items == nil {
		return raw, nil
	}
	for i, v := range items {
		pruned, err := pruneToSchema(v, elem)
		if err != nil {
			return nil, err
		}
		items[i] = pruned
	}
	return json.Marshal(items)
}
