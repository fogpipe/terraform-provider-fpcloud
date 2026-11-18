package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                   = &FirewallResource{}
	_ resource.ResourceWithConfigure      = &FirewallResource{}
	_ resource.ResourceWithImportState    = &FirewallResource{}
	_ resource.ResourceWithValidateConfig = &FirewallResource{}
)

// NewFirewallResource returns a new firewall resource.
func NewFirewallResource() resource.Resource {
	return &FirewallResource{}
}

// FirewallResource manages a project's firewall: an ordered access list on
// client addresses, attached to apps and bucket websites by their own
// `firewall` attribute. Its rules are declared inline and are the whole list.
type FirewallResource struct {
	client *client.Client
}

// FirewallResourceModel describes the resource data model.
type FirewallResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Project       types.String `tfsdk:"project"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	DefaultAction types.String `tfsdk:"default_action"`
	Rules         types.List   `tfsdk:"rules"`
	AllowLockout  types.Bool   `tfsdk:"allow_lockout"`
	Revision      types.Int64  `tfsdk:"revision"`
	Findings      types.List   `tfsdk:"findings"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

// FirewallRuleModel is one rule in state.
type FirewallRuleModel struct {
	Priority    types.Int64  `tfsdk:"priority"`
	Action      types.String `tfsdk:"action"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Match       types.Object `tfsdk:"match"`
}

// FirewallMatchModel is what a rule matches on, in state.
type FirewallMatchModel struct {
	Sources types.Set `tfsdk:"sources"`
}

func (r *FirewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

func (r *FirewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a firewall: a project's ordered access list on client addresses, enforced at the " +
			"platform's edge. Rules are evaluated in priority order, lowest first, the first match decides, " +
			"and a request no rule matches gets `default_action`. A firewall does nothing until it is attached: " +
			"set `firewall` on an fpcloud_app or on an fpcloud_bucket serving a website, and it applies to every " +
			"host the edge serves for that target, custom domains included. It covers requests arriving through " +
			"the edge and nothing else: not calls between apps inside the platform, and not outbound traffic. " +
			"The rules declared here are the whole list: a rule added outside this configuration is removed by " +
			"the next apply. A rule matches the address the edge sees the connection come from, so behind a CDN " +
			"or a proxy of your own the rules see that CDN's addresses, never the visitor's.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Firewall ID. What an app's or a bucket's `firewall` attribute takes.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "ID of the project this firewall belongs to. It attaches to apps and websites of " +
					"the same project. Changing it forces a new firewall.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Firewall name, unique in the project. Changing it forces a new firewall.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "What the firewall is for.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"default_action": schema.StringAttribute{
				Description: "What happens to a request no enabled rule matches: `allow` or `deny`. Defaults to " +
					"`allow`, so a firewall with no rules changes nothing. `deny` makes the rules an allowlist.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("allow"),
			},
			"rules": schema.ListNestedAttribute{
				Description: "The access rules, all of them. A rule is identified by its priority, and rules are " +
					"evaluated in priority order whatever order they are written in. At most 50 rules and 500 " +
					"sources across them.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"priority": schema.Int64Attribute{
							Description: "Evaluation order, lowest first, between 1 and 65535. Unique within the " +
								"firewall. Leave gaps so a rule can be added between two others later.",
							Required: true,
						},
						"action": schema.StringAttribute{
							Description: "`allow` or `deny`.",
							Required:    true,
						},
						"description": schema.StringAttribute{
							Description: "What the rule is for, at most 256 characters.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(""),
						},
						"enabled": schema.BoolAttribute{
							Description: "Whether the rule is evaluated. Defaults to true. A disabled rule is kept and skipped.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
						},
						"match": schema.SingleNestedAttribute{
							Description: "What the rule matches on.",
							Required:    true,
							Attributes: map[string]schema.Attribute{
								"sources": schema.SetAttribute{
									Description: "Client address ranges as CIDRs, IPv4 or IPv6; a request from any of " +
										"them matches. A bare address is the one-address range, so `10.0.0.5` and " +
										"`10.0.0.5/32` are the same source.",
									Required:    true,
									ElementType: cidrType{},
								},
							},
						},
					},
				},
			},
			"allow_lockout": schema.BoolAttribute{
				Description: "Admit a change that denies the address this apply runs from. Without it, a change " +
					"to an attached firewall that would shut out the caller is refused, which is what stops a " +
					"typo from closing a site to its owner. Set it when the machine running the apply is not " +
					"meant to reach the target, a CI runner for instance. Defaults to false. A setting of the " +
					"request, never stored by the API.",
				Optional: true,
			},
			"revision": schema.Int64Attribute{
				Description: "Counts the firewall's content: every change to the default action or the rules " +
					"stores the next one. An apply returns once every target enforces it.",
				Computed: true,
			},
			"findings": schema.ListNestedAttribute{
				Description: "Warnings about a firewall that is valid and enforced as written: a rule no request " +
					"can reach, a default of deny with no IPv6 source. Also shown as warnings when an apply " +
					"writes the firewall.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"code": schema.StringAttribute{
							Description: "What kind of warning it is.",
							Computed:    true,
						},
						"field": schema.StringAttribute{
							Description: "What it is about, as `rules[2]` or `default_action`.",
							Computed:    true,
						},
						"message": schema.StringAttribute{
							Description: "The warning.",
							Computed:    true,
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				Description: "When the firewall was created, RFC 3339.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "When the firewall last changed, RFC 3339.",
				Computed:    true,
			},
		},
	}
}

func (r *FirewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

// ValidateConfig refuses at plan what the API refuses at apply: an action that
// is neither allow nor deny, a priority two rules share, a source that is not
// an address range, and one range listed twice in a rule under two spellings.
// The last one matters most here: the API answers with one entry where the
// configuration wrote two, which no state could be consistent with.
func (r *FirewallResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg FirewallResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !cfg.DefaultAction.IsNull() && !cfg.DefaultAction.IsUnknown() && !isFirewallAction(cfg.DefaultAction.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("default_action"), "Invalid default action",
			fmt.Sprintf("default_action is %q; it must be \"allow\" or \"deny\".", cfg.DefaultAction.ValueString()))
	}
	if cfg.Rules.IsNull() || cfg.Rules.IsUnknown() {
		return
	}
	priorities := map[int64]int{}
	for i, elem := range cfg.Rules.Elements() {
		obj, ok := elem.(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		at := path.Root("rules").AtListIndex(i)
		attrs := obj.Attributes()
		if p, ok := attrs["priority"].(types.Int64); ok && !p.IsNull() && !p.IsUnknown() {
			if p.ValueInt64() < 1 || p.ValueInt64() > 65535 {
				resp.Diagnostics.AddAttributeError(at.AtName("priority"), "Invalid rule priority",
					fmt.Sprintf("priority is %d; it must be between 1 and 65535.", p.ValueInt64()))
			}
			if first, taken := priorities[p.ValueInt64()]; taken {
				resp.Diagnostics.AddAttributeError(at.AtName("priority"), "Duplicate rule priority",
					fmt.Sprintf("priority %d is already taken by rules[%d]; a priority identifies one rule.", p.ValueInt64(), first))
			} else {
				priorities[p.ValueInt64()] = i
			}
		}
		if a, ok := attrs["action"].(types.String); ok && !a.IsNull() && !a.IsUnknown() && !isFirewallAction(a.ValueString()) {
			resp.Diagnostics.AddAttributeError(at.AtName("action"), "Invalid rule action",
				fmt.Sprintf("action is %q; it must be \"allow\" or \"deny\".", a.ValueString()))
		}
		match, ok := attrs["match"].(types.Object)
		if !ok || match.IsNull() || match.IsUnknown() {
			continue
		}
		sources, ok := match.Attributes()["sources"].(types.Set)
		if !ok || sources.IsNull() || sources.IsUnknown() {
			continue
		}
		sourcesAt := at.AtName("match").AtName("sources")
		if len(sources.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(sourcesAt, "A rule names at least one source",
				"sources is empty; a rule that matches nothing is left out instead.")
		}
		seen := map[string]string{}
		for _, s := range sources.Elements() {
			written, known := sourceString(s)
			if !known {
				continue
			}
			canonical, err := canonicalCIDR(written)
			if err != nil {
				resp.Diagnostics.AddAttributeError(sourcesAt, "Invalid source", err.Error())
				continue
			}
			if other, dup := seen[canonical]; dup {
				resp.Diagnostics.AddAttributeError(sourcesAt, "Duplicate source",
					fmt.Sprintf("%q and %q are the same range, %s; list it once.", other, written, canonical))
				continue
			}
			seen[canonical] = written
		}
	}
}

func isFirewallAction(s string) bool { return s == "allow" || s == "deny" }

// sourceString reads one element of a rule's sources as the configuration
// wrote it. The second result is false for a null or unknown element.
func sourceString(v attr.Value) (string, bool) {
	s, ok := v.(basetypes.StringValuable)
	if !ok || v.IsNull() || v.IsUnknown() {
		return "", false
	}
	sv, diags := s.ToStringValue(context.Background())
	if diags.HasError() {
		return "", false
	}
	return sv.ValueString(), true
}

func (r *FirewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules := firewallRulesFromModel(ctx, plan.Rules, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	// A firewall is attached to nothing when it is created, so there is no
	// caller to lock out and no target to wait for.
	fw, err := r.client.CreateFirewall(ctx, plan.Project.ValueString(), client.CreateFirewallRequest{
		Name:          plan.Name.ValueString(),
		Description:   plan.Description.ValueString(),
		DefaultAction: plan.DefaultAction.ValueString(),
		Rules:         rules,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall", err.Error())
		return
	}
	warnFirewallFindings(fw, &resp.Diagnostics)
	setModelFromFirewall(&plan, fw, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fw, err := r.client.GetFirewall(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading firewall", err.Error())
		return
	}
	setModelFromFirewall(&state, fw, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state FirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules := firewallRulesFromModel(ctx, plan.Rules, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	fw, err := r.client.UpdateFirewall(ctx, id, client.UpdateFirewallRequest{
		Description:   plan.Description.ValueString(),
		DefaultAction: plan.DefaultAction.ValueString(),
		Rules:         rules,
		AllowLockout:  plan.AllowLockout.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating firewall", err.Error())
		return
	}
	warnFirewallFindings(fw, &resp.Diagnostics)

	// The firewall holds the new content from here on, whether or not the
	// edge takes it: record it before waiting, so a target that fails leaves
	// state saying what the API holds.
	setModelFromFirewall(&plan, fw, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := awaitFirewallTargets(ctx, func(ctx context.Context) (*client.Firewall, error) {
		return r.client.GetFirewall(ctx, id)
	}); err != nil {
		resp.Diagnostics.AddError("Error applying firewall", err.Error())
	}
}

func (r *FirewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteFirewall(ctx, state.ID.ValueString()); err != nil {
		if isNotFound(err) {
			return
		}
		// The API names what the firewall is still attached to; that is the
		// thing to change, so its message is the one shown.
		if isConflict(err) {
			resp.Diagnostics.AddError("Firewall is still attached", err.Error())
			return
		}
		resp.Diagnostics.AddError("Error deleting firewall", err.Error())
	}
}

// ImportState accepts a firewall id, or "project/name" where project is the
// project's id. The id is tried first; on a miss the value is resolved through
// the project's list.
func (r *FirewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if _, err := r.client.GetFirewall(ctx, id); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	} else if !isNotFound(err) && !strings.Contains(id, "/") {
		resp.Diagnostics.AddError("Error importing firewall", err.Error())
		return
	}
	project, name, ok := strings.Cut(id, "/")
	if !ok || project == "" || name == "" {
		resp.Diagnostics.AddError("Error importing firewall",
			fmt.Sprintf("%q is not a known firewall id; import by name takes \"project/name\"", id))
		return
	}
	firewalls, err := r.client.ListFirewalls(ctx, project)
	if err != nil {
		resp.Diagnostics.AddError("Error importing firewall", err.Error())
		return
	}
	for _, fw := range firewalls {
		if fw.Name == name {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fw.ID)...)
			return
		}
	}
	resp.Diagnostics.AddError("Error importing firewall",
		fmt.Sprintf("firewall %q in project %q is %s", name, project, errNotAccessible))
}

// warnFirewallFindings shows the API's warnings about the firewall it just
// stored. The firewall is valid and enforced as written; a finding says it
// may not do what its author meant.
func warnFirewallFindings(fw *client.Firewall, diags *diag.Diagnostics) {
	for _, f := range fw.Findings {
		diags.AddAttributeWarning(path.Root("rules"), "Firewall "+fw.Name+": "+f.Field, f.Message)
	}
}

func firewallMatchAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"sources": types.SetType{ElemType: cidrType{}},
	}
}

func firewallRuleAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"priority":    types.Int64Type,
		"action":      types.StringType,
		"description": types.StringType,
		"enabled":     types.BoolType,
		"match":       types.ObjectType{AttrTypes: firewallMatchAttrTypes()},
	}
}

func firewallFindingAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"code":    types.StringType,
		"field":   types.StringType,
		"message": types.StringType,
	}
}

// firewallRulesFromModel reads the rules as the configuration wrote them. The
// API canonicalises the sources and orders the rules; nothing is rewritten on
// the way out. No rules is an empty list, which is what replaces a firewall's
// rules with none.
func firewallRulesFromModel(ctx context.Context, l types.List, diags *diag.Diagnostics) []client.FirewallRuleRequest {
	out := []client.FirewallRuleRequest{}
	if l.IsNull() || l.IsUnknown() {
		return out
	}
	var rules []FirewallRuleModel
	diags.Append(l.ElementsAs(ctx, &rules, false)...)
	if diags.HasError() {
		return out
	}
	for _, rule := range rules {
		var match FirewallMatchModel
		diags.Append(rule.Match.As(ctx, &match, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return out
		}
		sources := []string{}
		for _, s := range match.Sources.Elements() {
			if written, ok := sourceString(s); ok {
				sources = append(sources, written)
			}
		}
		enabled := rule.Enabled.ValueBool()
		out = append(out, client.FirewallRuleRequest{
			Priority:    int(rule.Priority.ValueInt64()),
			Action:      rule.Action.ValueString(),
			Description: rule.Description.ValueString(),
			Enabled:     &enabled,
			Match:       client.FirewallMatch{Sources: sources},
		})
	}
	return out
}

// firewallRulesToList writes the API's rules in the order the model already
// holds them. The API answers in priority order and the configuration may be
// written in any order, so a rule keeps the position its priority had; one the
// model has never seen follows, in priority order. Sources are written the way
// the API spells them: cidrType keeps the configuration's spelling of the same
// range.
func firewallRulesToList(prior types.List, rules []client.FirewallRule, diags *diag.Diagnostics) types.List {
	ruleType := types.ObjectType{AttrTypes: firewallRuleAttrTypes()}
	if len(rules) == 0 {
		// No rules is written by leaving the attribute out or as `[]`; the one
		// the configuration chose is kept.
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior
		}
		return types.ListNull(ruleType)
	}

	byPriority := make(map[int64]client.FirewallRule, len(rules))
	for _, rule := range rules {
		byPriority[int64(rule.Priority)] = rule
	}
	ordered := make([]client.FirewallRule, 0, len(rules))
	if !prior.IsNull() && !prior.IsUnknown() {
		for _, elem := range prior.Elements() {
			obj, ok := elem.(types.Object)
			if !ok {
				continue
			}
			p, ok := obj.Attributes()["priority"].(types.Int64)
			if !ok || p.IsNull() || p.IsUnknown() {
				continue
			}
			if rule, found := byPriority[p.ValueInt64()]; found {
				ordered = append(ordered, rule)
				delete(byPriority, p.ValueInt64())
			}
		}
	}
	for _, rule := range rules {
		if _, left := byPriority[int64(rule.Priority)]; left {
			ordered = append(ordered, rule)
		}
	}

	elems := make([]attr.Value, 0, len(ordered))
	for _, rule := range ordered {
		sources := make([]attr.Value, 0, len(rule.Match.Sources))
		for _, s := range rule.Match.Sources {
			sources = append(sources, newCIDRValue(s))
		}
		set, d := types.SetValue(cidrType{}, sources)
		diags.Append(d...)
		match, d := types.ObjectValue(firewallMatchAttrTypes(), map[string]attr.Value{"sources": set})
		diags.Append(d...)
		obj, d := types.ObjectValue(firewallRuleAttrTypes(), map[string]attr.Value{
			"priority":    types.Int64Value(int64(rule.Priority)),
			"action":      types.StringValue(rule.Action),
			"description": types.StringValue(rule.Description),
			"enabled":     types.BoolValue(rule.Enabled),
			"match":       match,
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(ruleType, elems)
	diags.Append(d...)
	return list
}

// setModelFromFirewall copies what the API reports. allow_lockout is a setting
// of the request and is never read back, so it stays what the model holds.
func setModelFromFirewall(m *FirewallResourceModel, fw *client.Firewall, diags *diag.Diagnostics) {
	m.ID = types.StringValue(fw.ID)
	// Keep the configured project when the API does not echo one, so a
	// required attribute never turns to "" after an apply.
	if fw.ProjectID != "" {
		m.Project = types.StringValue(fw.ProjectID)
	}
	m.Name = types.StringValue(fw.Name)
	m.Description = types.StringValue(fw.Description)
	m.DefaultAction = types.StringValue(fw.DefaultAction)
	m.Rules = firewallRulesToList(m.Rules, fw.Rules, diags)
	m.Revision = types.Int64Value(int64(fw.Revision))

	findingType := types.ObjectType{AttrTypes: firewallFindingAttrTypes()}
	findings := make([]attr.Value, 0, len(fw.Findings))
	for _, f := range fw.Findings {
		obj, d := types.ObjectValue(firewallFindingAttrTypes(), map[string]attr.Value{
			"code":    types.StringValue(f.Code),
			"field":   types.StringValue(f.Field),
			"message": types.StringValue(f.Message),
		})
		diags.Append(d...)
		findings = append(findings, obj)
	}
	list, d := types.ListValue(findingType, findings)
	diags.Append(d...)
	m.Findings = list

	m.CreatedAt = types.StringValue(fw.CreatedAt.UTC().Format(time.RFC3339))
	m.UpdatedAt = types.StringValue(fw.UpdatedAt.UTC().Format(time.RFC3339))
}

// firewallPoll is how often a wait for the edge reads again, and
// firewallTimeout how long it reads for.
var (
	firewallPoll    = 2 * time.Second
	firewallTimeout = 5 * time.Minute
)

// awaitFirewallTargets waits until every app and website the firewall is
// attached to enforces its current revision. A change is stored at once and
// rendered to the edge afterwards, so the API's answer to a write says what
// was asked for, not what requests meet; an apply that returned on the answer
// would let whatever runs next assume a rule that is not in force yet. It
// fails with the target's message when the edge refuses the revision, and
// returns at once for a firewall attached to nothing.
func awaitFirewallTargets(ctx context.Context, read func(context.Context) (*client.Firewall, error)) (*client.Firewall, error) {
	deadline := time.Now().Add(firewallTimeout)
	for {
		fw, err := read(ctx)
		if err != nil {
			return nil, err
		}
		pending := ""
		for _, t := range fw.Targets {
			if t.Status == "failed" {
				return fw, fmt.Errorf("the edge did not apply revision %d of firewall %s to %s %s: %s",
					fw.Revision, fw.Name, t.Type, t.Name, orNoMessage(t.Message))
			}
			if pending == "" && (t.Status != "applied" || t.AppliedRevision != fw.Revision) {
				pending = t.Type + " " + t.Name
			}
		}
		if pending == "" {
			return fw, nil
		}
		if time.Now().After(deadline) {
			return fw, fmt.Errorf("timed out after %s waiting for %s to enforce revision %d of firewall %s; the change is stored and the edge may still take it",
				firewallTimeout, pending, fw.Revision, fw.Name)
		}
		select {
		case <-ctx.Done():
			return fw, ctx.Err()
		case <-time.After(firewallPoll):
		}
	}
}

// awaitAttachedFirewall waits until a target's own read says the edge enforces
// what was asked of it: firewall want at its current revision, or no firewall
// at all when want is empty. read returns the target's Firewall, nil when it
// has none; what names the target in an error.
func awaitAttachedFirewall(ctx context.Context, what, want string, read func(context.Context) (*client.FirewallRef, error)) error {
	deadline := time.Now().Add(firewallTimeout)
	for {
		ref, err := read(ctx)
		if err != nil {
			return err
		}
		switch {
		case ref == nil:
			if want == "" {
				return nil
			}
		case ref.Status == "failed":
			if want == "" {
				return fmt.Errorf("the edge did not take the firewall off %s: %s", what, orNoMessage(ref.Message))
			}
			return fmt.Errorf("the edge did not apply the firewall to %s: %s", what, orNoMessage(ref.Message))
		case want != "" && ref.ID == want && ref.Status == "applied" && ref.AppliedRevision == ref.Revision:
			return nil
		}
		if time.Now().After(deadline) {
			if want == "" {
				return fmt.Errorf("timed out after %s waiting for the edge to take the firewall off %s", firewallTimeout, what)
			}
			return fmt.Errorf("timed out after %s waiting for the edge to enforce the firewall on %s; the attachment is stored and the edge may still take it", firewallTimeout, what)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(firewallPoll):
		}
	}
}

func orNoMessage(msg string) string {
	if msg == "" {
		return "the API gave no reason"
	}
	return msg
}

// attachedFirewallID reads a target's firewall as the `firewall` attribute
// holds it: the id, or null for none. A firewall still being taken off the
// edge has no id and reads as none, which is what was asked for.
func attachedFirewallID(ref *client.FirewallRef) types.String {
	if ref == nil || ref.ID == "" {
		return types.StringNull()
	}
	return types.StringValue(ref.ID)
}
