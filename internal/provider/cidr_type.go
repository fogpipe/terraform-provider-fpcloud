package provider

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// canonicalCIDR is the one spelling the API keeps for an address range: the
// masked prefix, a bare address being the one-address range. `10.0.0.5` is
// `10.0.0.5/32`, `10.1.2.3/8` is `10.0.0.0/8`, and an IPv6 range is written
// the short way. An IPv4-mapped IPv6 range is refused the way the API refuses
// it: no client address matches one.
func canonicalCIDR(s string) (string, error) {
	if s == "" || s != strings.TrimSpace(s) {
		return "", fmt.Errorf("%q is not a CIDR or an address", s)
	}
	var p netip.Prefix
	if strings.Contains(s, "/") {
		parsed, err := netip.ParsePrefix(s)
		if err != nil {
			return "", fmt.Errorf("%q is not a CIDR", s)
		}
		p = parsed
	} else {
		addr, err := netip.ParseAddr(s)
		if err != nil || addr.Zone() != "" {
			return "", fmt.Errorf("%q is not a CIDR or an address", s)
		}
		p = netip.PrefixFrom(addr, addr.BitLen())
	}
	if p.Addr().Is4In6() {
		return "", fmt.Errorf("%q is an IPv4-mapped IPv6 range, which no client address matches; write it as IPv4", s)
	}
	return p.Masked().String(), nil
}

var (
	_ basetypes.StringTypable                    = cidrType{}
	_ basetypes.StringValuableWithSemanticEquals = cidrValue{}
)

// cidrType is a string holding an address range. Two values naming the same
// range are semantically equal, so the spelling the configuration wrote stays
// in state when the API answers with its canonical one, and a bare address is
// not a difference from its /32 on every plan.
type cidrType struct {
	basetypes.StringType
}

func (t cidrType) String() string { return "cidrType" }

func (t cidrType) Equal(o attr.Type) bool {
	other, ok := o.(cidrType)
	return ok && t.StringType.Equal(other.StringType)
}

func (t cidrType) ValueType(context.Context) attr.Value { return cidrValue{} }

func (t cidrType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return cidrValue{StringValue: in}, nil
}

func (t cidrType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return cidrValue{StringValue: s}, nil
}

// cidrValue is one address range as the configuration or the API spelled it.
type cidrValue struct {
	basetypes.StringValue
}

func newCIDRValue(s string) cidrValue {
	return cidrValue{StringValue: basetypes.NewStringValue(s)}
}

func (v cidrValue) Type(context.Context) attr.Type { return cidrType{} }

func (v cidrValue) Equal(o attr.Value) bool {
	other, ok := o.(cidrValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals reports whether both values name the same range. A
// value that does not parse equals nothing: the API refuses it, and that
// refusal is the message to show.
func (v cidrValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	o, ok := other.(cidrValue)
	if !ok {
		return false, nil
	}
	a, errA := canonicalCIDR(v.ValueString())
	b, errB := canonicalCIDR(o.ValueString())
	return errA == nil && errB == nil && a == b, nil
}
