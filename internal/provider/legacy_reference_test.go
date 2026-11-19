package provider_test

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// legacyReference is one reference as a schema before it was kept as written
// served it: the input `old` held it, and neither `attr` nor its computed
// `partner` existed.
type legacyReference struct {
	attr, partner, old string
}

// legacyProvider serves this provider with one resource type in the shape it
// had at schema version `version`, before its references were kept as written,
// so that a step run with it writes state the way that release did and the
// next step, run with the current provider, has to upgrade it. Every request
// for the type is translated to the current shape and every answer back; the
// rest of the provider is served as it is.
//
// The old input is answered with the reference as the configuration wrote it.
// The release this stands in for wrote the id the API answered instead, so a
// state it wrote is reproduced exactly only for a configuration that names
// the referenced resource by its id, which is the only configuration that
// release could apply.
func legacyProvider(typeName string, version int64, refs ...legacyReference) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"fpcloud": func() (tfprotov6.ProviderServer, error) {
			inner, err := providerserver.NewProtocol6WithError(provider.New("test")())()
			if err != nil {
				return nil, err
			}
			return &legacyServer{ProviderServer: inner, typeName: typeName, version: version, refs: refs}, nil
		},
	}
}

type legacyServer struct {
	tfprotov6.ProviderServer
	typeName string
	version  int64
	refs     []legacyReference

	current, old tftypes.Type
}

func (s *legacyServer) GetProviderSchema(ctx context.Context, req *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	resp, err := s.ProviderServer.GetProviderSchema(ctx, req)
	if err != nil || resp == nil {
		return resp, err
	}
	sch, ok := resp.ResourceSchemas[s.typeName]
	if !ok {
		return nil, fmt.Errorf("%s is not a resource this provider serves", s.typeName)
	}
	s.current = sch.ValueType()
	block := *sch.Block
	byName := map[string]*tfprotov6.SchemaAttribute{}
	for _, a := range sch.Block.Attributes {
		byName[a.Name] = a
	}
	gone := map[string]bool{}
	var added []*tfprotov6.SchemaAttribute
	for _, r := range s.refs {
		gone[r.attr], gone[r.partner] = true, true
		input := *byName[r.attr]
		input.Name = r.old
		added = append(added, &input)
	}
	block.Attributes = nil
	for _, a := range sch.Block.Attributes {
		if !gone[a.Name] {
			block.Attributes = append(block.Attributes, a)
		}
	}
	block.Attributes = append(block.Attributes, added...)
	legacy := &tfprotov6.Schema{Version: s.version, Block: &block}
	s.old = legacy.ValueType()

	schemas := make(map[string]*tfprotov6.Schema, len(resp.ResourceSchemas))
	for name, sch := range resp.ResourceSchemas {
		schemas[name] = sch
	}
	schemas[s.typeName] = legacy
	out := *resp
	out.ResourceSchemas = schemas
	return &out, nil
}

// partnerFrom says what a translated value's partner holds: the old input's
// value in a state, nothing in a configuration, and in a proposed new state
// what the prior state held.
type partnerFrom int

const (
	partnerIsInput partnerFrom = iota
	partnerIsNull
	partnerIsPrior
)

// toCurrent translates a value of the old shape to the current one.
func (s *legacyServer) toCurrent(dv *tfprotov6.DynamicValue, from partnerFrom, prior *tfprotov6.DynamicValue) (*tfprotov6.DynamicValue, error) {
	if dv == nil {
		return nil, nil
	}
	v, err := dv.Unmarshal(s.old)
	if err != nil {
		return nil, err
	}
	var priorAttrs map[string]tftypes.Value
	if from == partnerIsPrior && prior != nil {
		p, err := prior.Unmarshal(s.old)
		if err != nil {
			return nil, err
		}
		if !p.IsNull() {
			if err := p.As(&priorAttrs); err != nil {
				return nil, err
			}
		}
	}
	if v.IsNull() {
		out, err := tfprotov6.NewDynamicValue(s.current, tftypes.NewValue(s.current, nil))
		return &out, err
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		return nil, err
	}
	for _, r := range s.refs {
		input := attrs[r.old]
		delete(attrs, r.old)
		attrs[r.attr] = input
		switch {
		case from == partnerIsInput:
			attrs[r.partner] = input
		case from == partnerIsPrior && priorAttrs != nil:
			attrs[r.partner] = priorAttrs[r.old]
		default:
			attrs[r.partner] = tftypes.NewValue(tftypes.String, nil)
		}
	}
	out, err := tfprotov6.NewDynamicValue(s.current, tftypes.NewValue(s.current, attrs))
	return &out, err
}

// toOld translates a value of the current shape back to the old one.
func (s *legacyServer) toOld(dv *tfprotov6.DynamicValue) (*tfprotov6.DynamicValue, error) {
	if dv == nil {
		return nil, nil
	}
	v, err := dv.Unmarshal(s.current)
	if err != nil {
		return nil, err
	}
	if v.IsNull() {
		out, err := tfprotov6.NewDynamicValue(s.old, tftypes.NewValue(s.old, nil))
		return &out, err
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		return nil, err
	}
	for _, r := range s.refs {
		input := attrs[r.attr]
		delete(attrs, r.attr)
		delete(attrs, r.partner)
		attrs[r.old] = input
	}
	out, err := tfprotov6.NewDynamicValue(s.old, tftypes.NewValue(s.old, attrs))
	return &out, err
}

func (s *legacyServer) UpgradeResourceState(ctx context.Context, req *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	if req.TypeName != s.typeName {
		return s.ProviderServer.UpgradeResourceState(ctx, req)
	}
	v, err := req.RawState.UnmarshalWithOpts(s.old, tfprotov6.UnmarshalOpts{})
	if err != nil {
		return nil, err
	}
	out, err := tfprotov6.NewDynamicValue(s.old, v)
	if err != nil {
		return nil, err
	}
	return &tfprotov6.UpgradeResourceStateResponse{UpgradedState: &out}, nil
}

func (s *legacyServer) ValidateResourceConfig(ctx context.Context, req *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	if req.TypeName != s.typeName {
		return s.ProviderServer.ValidateResourceConfig(ctx, req)
	}
	r := *req
	var err error
	if r.Config, err = s.toCurrent(req.Config, partnerIsNull, nil); err != nil {
		return nil, err
	}
	return s.ProviderServer.ValidateResourceConfig(ctx, &r)
}

func (s *legacyServer) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	if req.TypeName != s.typeName {
		return s.ProviderServer.ReadResource(ctx, req)
	}
	r := *req
	var err error
	if r.CurrentState, err = s.toCurrent(req.CurrentState, partnerIsInput, nil); err != nil {
		return nil, err
	}
	resp, err := s.ProviderServer.ReadResource(ctx, &r)
	if err != nil || resp == nil {
		return resp, err
	}
	out := *resp
	if out.NewState, err = s.toOld(resp.NewState); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *legacyServer) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	if req.TypeName != s.typeName {
		return s.ProviderServer.PlanResourceChange(ctx, req)
	}
	r := *req
	var err error
	if r.PriorState, err = s.toCurrent(req.PriorState, partnerIsInput, nil); err != nil {
		return nil, err
	}
	if r.ProposedNewState, err = s.toCurrent(req.ProposedNewState, partnerIsPrior, req.PriorState); err != nil {
		return nil, err
	}
	if r.Config, err = s.toCurrent(req.Config, partnerIsNull, nil); err != nil {
		return nil, err
	}
	resp, err := s.ProviderServer.PlanResourceChange(ctx, &r)
	if err != nil || resp == nil {
		return resp, err
	}
	out := *resp
	if out.PlannedState, err = s.toOld(resp.PlannedState); err != nil {
		return nil, err
	}
	out.RequiresReplace = nil
	for _, p := range resp.RequiresReplace {
		out.RequiresReplace = append(out.RequiresReplace, s.oldPath(p))
	}
	return &out, nil
}

// oldPath names a top-level reference or its partner by the old input.
func (s *legacyServer) oldPath(p *tftypes.AttributePath) *tftypes.AttributePath {
	steps := p.Steps()
	if len(steps) == 0 {
		return p
	}
	name, ok := steps[0].(tftypes.AttributeName)
	if !ok {
		return p
	}
	for _, r := range s.refs {
		if string(name) == r.attr || string(name) == r.partner {
			return tftypes.NewAttributePathWithSteps(append([]tftypes.AttributePathStep{tftypes.AttributeName(r.old)}, steps[1:]...))
		}
	}
	return p
}

func (s *legacyServer) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	if req.TypeName != s.typeName {
		return s.ProviderServer.ApplyResourceChange(ctx, req)
	}
	r := *req
	var err error
	if r.PriorState, err = s.toCurrent(req.PriorState, partnerIsInput, nil); err != nil {
		return nil, err
	}
	if r.PlannedState, err = s.toCurrent(req.PlannedState, partnerIsPrior, req.PriorState); err != nil {
		return nil, err
	}
	if r.Config, err = s.toCurrent(req.Config, partnerIsNull, nil); err != nil {
		return nil, err
	}
	resp, err := s.ProviderServer.ApplyResourceChange(ctx, &r)
	if err != nil || resp == nil {
		return resp, err
	}
	out := *resp
	if out.NewState, err = s.toOld(resp.NewState); err != nil {
		return nil, err
	}
	return &out, nil
}
