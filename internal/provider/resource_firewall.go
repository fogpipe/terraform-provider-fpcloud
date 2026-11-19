package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
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
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var (
	_ resource.Resource                   = &FirewallResource{}
	_ resource.ResourceWithConfigure      = &FirewallResource{}
	_ resource.ResourceWithImportState    = &FirewallResource{}
	_ resource.ResourceWithValidateConfig = &FirewallResource{}
	_ resource.ResourceWithModifyPlan     = &FirewallResource{}
	_ resource.ResourceWithUpgradeState   = &FirewallResource{}
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
	ProjectID     types.String `tfsdk:"project_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	DefaultAction types.String `tfsdk:"default_action"`
	Rules         types.Map    `tfsdk:"rules"`
	Revision      types.Int64  `tfsdk:"revision"`
	Findings      types.List   `tfsdk:"findings"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

// FirewallRuleModel is one rule in state. Its priority is its key in `rules`.
type FirewallRuleModel struct {
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
		// Version 1 keys `rules` by priority; version 0 held them as a list
		// with the priority inside each rule (UpgradeState).
		Version: 1,
		Description: "Manages a firewall: a project's ordered access list on client addresses, enforced at the " +
			"platform's edge. Rules are evaluated in priority order, lowest first, the first match decides, " +
			"and a request no rule matches gets `default_action`. A firewall does nothing until it is attached: " +
			"set `firewall` on an fpcloud_app or on an fpcloud_bucket serving a website, and it applies to every " +
			"host the edge serves for that target, custom domains included. It covers requests arriving through " +
			"the edge and nothing else: not calls between apps inside the platform, and not outbound traffic. " +
			"The rules declared here are the whole list: a rule added outside this configuration is removed by " +
			"the next apply. An apply writes the firewall only if it is still at the revision its plan read, so " +
			"a change made elsewhere after the plan fails the apply rather than being overwritten unseen. " +
			"A rule matches the address the edge sees the connection come from, so behind a CDN or a proxy " +
			"of your own the rules see that CDN's addresses, never the visitor's.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Firewall ID. An app's or a bucket's `firewall` attribute takes it, or the firewall's name.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "The project, by name or id, this firewall belongs to. It attaches to apps and " +
					"websites of the same project. It records the reference as written: another spelling of " +
					"the same project changes nothing, and only a different project forces a new firewall.",
				Required: true,
			},
			"project_id": schema.StringAttribute{
				Description: "The project's id. This is what a plan compares: the firewall is replaced when " +
					"it changes, never when `project` is merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
			"rules": schema.MapNestedAttribute{
				Description: "The access rules, all of them, keyed by priority: `rules = { 100 = { ... } }`. " +
					"The key is the rule's identifier and its evaluation order, lowest first: a whole number " +
					"from 1 to 65535, written without a sign or leading zeros. Rules are evaluated in priority " +
					"order whatever order they are written in. Leave gaps so a rule can be added between two " +
					"others later; adding one plans that rule alone. At most 50 rules and 500 sources across them.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
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
			"revision": schema.Int64Attribute{
				Description: "Counts the firewall's content: every change to the default action or the rules " +
					"stores the next one. An apply returns once every target enforces it.",
				Computed: true,
			},
			"findings": schema.ListNestedAttribute{
				Description: "Warnings about a firewall that is valid and enforced as written: a rule no request " +
					"can reach, a default of deny with no IPv6 source. Also shown as warnings when an apply " +
					"writes the firewall. The warning that a change denies the address the apply ran from is " +
					"shown on that apply only and is not kept here, since it is about the machine and not " +
					"about the firewall.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"code": schema.StringAttribute{
							Description: "What kind of warning it is.",
							Computed:    true,
						},
						"field": schema.StringAttribute{
							Description: "What it is about, as the API names it: `rules[2]` is the third rule in priority " +
								"order, or `default_action`; empty when it is about the firewall as a whole.",
							Computed: true,
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
// is neither allow nor deny, a key that is not a priority, a source that is
// not an address range, and one range listed twice in a rule under two
// spellings. The last one matters most here: the API answers with one entry
// where the configuration wrote two, which no state could be consistent with.
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
	elems := cfg.Rules.Elements()
	keys := make([]string, 0, len(elems))
	for key := range elems {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		at := path.Root("rules").AtMapKey(key)
		if _, err := parseRulePriority(key); err != nil {
			resp.Diagnostics.AddAttributeError(at, "Invalid rule priority", err.Error())
		}
		obj, ok := elems[key].(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		attrs := obj.Attributes()
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

// canonicalPriority is a priority as a key is written: a decimal number with
// no sign, space or leading zero. One spelling per priority is what makes two
// keys two rules: `100` and `0100` would otherwise both name rule 100, and
// nothing could say which of the two the API holds.
var canonicalPriority = regexp.MustCompile(`^[1-9][0-9]*$`)

// parseRulePriority reads a key of `rules` as the priority it names, refusing
// any spelling but the canonical one and any number outside 1 to 65535.
func parseRulePriority(key string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(key))
	switch {
	case errors.Is(err, strconv.ErrRange), err == nil && (n < 1 || n > 65535):
		return 0, fmt.Errorf("the rule keyed %q is out of range: a priority is between 1 and 65535", key)
	case err != nil:
		return 0, fmt.Errorf("the rule keyed %q has no priority: a rule's key is its priority, a whole number from 1 to "+
			"65535 written with no sign, space or leading zero, as 100", key)
	case !canonicalPriority.MatchString(key):
		return 0, fmt.Errorf("the rule keyed %q is priority %d: write its key as %d, with no sign, space or leading zero, "+
			"so that one priority has one spelling", key, n, n)
	}
	return n, nil
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

// ModifyPlan decides whether a changed `project` is a different project; see
// planProjectReference.
func (r *FirewallResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state, config FirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, replace := planProjectReference(ctx, r.client, config.Project, state.Project, state.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	if replace {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("project"))
	}
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

	// A plan that respells only `project` changes nothing the API holds, and
	// writing the content again would be a write nobody asked for. Every
	// computed attribute is unknown in the plan all the same, so the firewall
	// is read and that is what reaches state.
	if plan.Description.Equal(state.Description) && plan.DefaultAction.Equal(state.DefaultAction) && plan.Rules.Equal(state.Rules) {
		fw, err := r.client.GetFirewall(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Error reading firewall", err.Error())
			return
		}
		setModelFromFirewall(&plan, fw, &resp.Diagnostics)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	// The configuration is the whole of the firewall and the apply wins over
	// an edit made elsewhere, but only over one its plan showed: the write is
	// conditional on the revision the plan read, so a change made after it is
	// refused rather than reverted without ever appearing in a plan.
	fw, err := r.client.UpdateFirewall(ctx, id, client.UpdateFirewallRequest{
		Description:   plan.Description.ValueString(),
		DefaultAction: plan.DefaultAction.ValueString(),
		Rules:         rules,
		IfRevision:    int(state.Revision.ValueInt64()),
	})
	if err != nil {
		if isFirewallRevisionConflict(err) {
			resp.Diagnostics.AddError("Firewall changed since the plan",
				fmt.Sprintf("Firewall %s was changed outside this configuration after this plan read it at revision %d, "+
					"so the plan does not show what this apply would overwrite. Nothing was written. Run a new plan to "+
					"see the change, then apply that. The API said: %s",
					state.Name.ValueString(), state.Revision.ValueInt64(), err))
			return
		}
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
// project's name or id. The id is tried first; on a miss the value is resolved
// through the project's list, and `project` records the project as the import
// spelled it, so a configuration written the same way plans no change.
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
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project"), project)...)
			return
		}
	}
	resp.Diagnostics.AddError("Error importing firewall",
		fmt.Sprintf("firewall %q in project %q is %s", name, project, errNotAccessible))
}

// findingDeniesCaller is the finding a write carries when the firewall it
// stored or attached denies the address the request came from. It is about
// the machine running the apply, so it is a warning on that apply and never
// part of the firewall's state.
const findingDeniesCaller = "denies_caller"

// isFirewallRevisionConflict reports a conditional write refused because the
// firewall had moved past the revision it was made to.
func isFirewallRevisionConflict(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict && apiErr.Code == "FIREWALL_REVISION_CONFLICT"
}

// findingRule is how a finding's field names one rule: its index among the
// firewall's rules in priority order, as the API answers them.
var findingRule = regexp.MustCompile(`^rules\[(\d+)\]$`)

// warnFirewallFindings shows the API's warnings about the firewall it just
// stored. The firewall is valid and enforced as written; a finding says it
// may not do what its author meant. One about a single rule is raised on that
// rule's key, since `rules[1]` would read as the rule keyed 1.
func warnFirewallFindings(fw *client.Firewall, diags *diag.Diagnostics) {
	for _, f := range fw.Findings {
		at, about := path.Root("rules"), f.Field
		if m := findingRule.FindStringSubmatch(f.Field); m != nil {
			if i, err := strconv.Atoi(m[1]); err == nil && i < len(fw.Rules) {
				key := strconv.Itoa(fw.Rules[i].Priority)
				at, about = at.AtMapKey(key), "rule "+key
			}
		}
		title := "Firewall " + fw.Name
		if about != "" {
			title += ": " + about
		}
		diags.AddAttributeWarning(at, title, f.Message)
	}
}

// warnFindings raises each finding as a warning on the attribute that caused
// it, titled with the firewall and the field the finding is about.
func warnFindings(attribute path.Path, firewall string, findings []client.FirewallFinding, diags *diag.Diagnostics) {
	for _, f := range findings {
		title := "Firewall " + firewall
		if f.Field != "" {
			title += ": " + f.Field
		}
		diags.AddAttributeWarning(attribute, title, f.Message)
	}
}

func firewallMatchAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"sources": types.SetType{ElemType: cidrType{}},
	}
}

func firewallRuleAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
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

// firewallRulesFromModel reads the rules as the configuration wrote them, in
// priority order. The API canonicalises the sources; nothing is rewritten on
// the way out. No rules is an empty list, which is what replaces a firewall's
// rules with none.
func firewallRulesFromModel(ctx context.Context, m types.Map, diags *diag.Diagnostics) []client.FirewallRuleRequest {
	out := []client.FirewallRuleRequest{}
	if m.IsNull() || m.IsUnknown() {
		return out
	}
	var rules map[string]FirewallRuleModel
	diags.Append(m.ElementsAs(ctx, &rules, false)...)
	if diags.HasError() {
		return out
	}
	for key, rule := range rules {
		priority, err := parseRulePriority(key)
		if err != nil {
			diags.AddAttributeError(path.Root("rules").AtMapKey(key), "Invalid rule priority", err.Error())
			return out
		}
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
			Priority:    priority,
			Action:      rule.Action.ValueString(),
			Description: rule.Description.ValueString(),
			Enabled:     &enabled,
			Match:       client.FirewallMatch{Sources: sources},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

// firewallRulesToMap writes the API's rules keyed by priority. Sources are
// written the way the API spells them: cidrType keeps the configuration's
// spelling of the same range.
func firewallRulesToMap(prior types.Map, rules []client.FirewallRule, diags *diag.Diagnostics) types.Map {
	ruleType := types.ObjectType{AttrTypes: firewallRuleAttrTypes()}
	if len(rules) == 0 {
		// No rules is written by leaving the attribute out or as `{}`; the one
		// the configuration chose is kept.
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior
		}
		return types.MapNull(ruleType)
	}
	elems := make(map[string]attr.Value, len(rules))
	for _, rule := range rules {
		sources := make([]attr.Value, 0, len(rule.Match.Sources))
		for _, s := range rule.Match.Sources {
			sources = append(sources, newCIDRValue(s))
		}
		set, d := types.SetValue(cidrType{}, sources)
		diags.Append(d...)
		match, d := types.ObjectValue(firewallMatchAttrTypes(), map[string]attr.Value{"sources": set})
		diags.Append(d...)
		obj, d := types.ObjectValue(firewallRuleAttrTypes(), map[string]attr.Value{
			"action":      types.StringValue(rule.Action),
			"description": types.StringValue(rule.Description),
			"enabled":     types.BoolValue(rule.Enabled),
			"match":       match,
		})
		diags.Append(d...)
		elems[strconv.Itoa(rule.Priority)] = obj
	}
	m, d := types.MapValue(ruleType, elems)
	diags.Append(d...)
	return m
}

// UpgradeState moves a state written against schema version 0, where `rules`
// was a list with each rule's priority inside it, to the map keyed by
// priority. It is mechanical and reads nothing from the API: the refresh that
// follows does that.
func (r *FirewallResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: upgradeFirewallStateV0},
	}
}

func upgradeFirewallStateV0(_ context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
		resp.Diagnostics.AddError("Error reading prior firewall state", err.Error())
		return
	}
	if err := upgradeFirewallRulesV0(raw); err != nil {
		resp.Diagnostics.AddError("Error upgrading prior firewall state", err.Error())
		return
	}
	out, err := json.Marshal(raw)
	if err != nil {
		resp.Diagnostics.AddError("Error writing upgraded firewall state", err.Error())
		return
	}
	resp.DynamicValue = &tfprotov6.DynamicValue{JSON: out}
}

// upgradeFirewallRulesV0 rewrites version 0's `rules`, a list of rules each
// carrying `priority`, as a map from that priority to the rule without it.
// Null stays null and an empty list becomes an empty map, which is how the
// configuration that wrote `[]` now spells it.
func upgradeFirewallRulesV0(raw map[string]json.RawMessage) error {
	prior, ok := raw["rules"]
	if !ok || string(prior) == "null" {
		return nil
	}
	var rules []map[string]json.RawMessage
	if err := json.Unmarshal(prior, &rules); err != nil {
		return fmt.Errorf("rules: %w", err)
	}
	keyed := make(map[string]map[string]json.RawMessage, len(rules))
	for i, rule := range rules {
		var priority json.Number
		if err := json.Unmarshal(rule["priority"], &priority); err != nil {
			return fmt.Errorf("rules[%d].priority: %w", i, err)
		}
		n, err := strconv.Atoi(priority.String())
		if err != nil {
			return fmt.Errorf("rules[%d].priority is %s, not a whole number", i, priority)
		}
		key := strconv.Itoa(n)
		if _, taken := keyed[key]; taken {
			return fmt.Errorf("rules[%d]: priority %d is held by another rule", i, n)
		}
		delete(rule, "priority")
		keyed[key] = rule
	}
	out, err := json.Marshal(keyed)
	if err != nil {
		return err
	}
	raw["rules"] = out
	return nil
}

// setModelFromFirewall copies what the API reports.
func setModelFromFirewall(m *FirewallResourceModel, fw *client.Firewall, diags *diag.Diagnostics) {
	m.ID = types.StringValue(fw.ID)
	setProjectReference(&m.Project, &m.ProjectID, fw.ProjectID)
	m.Name = types.StringValue(fw.Name)
	m.Description = types.StringValue(fw.Description)
	m.DefaultAction = types.StringValue(fw.DefaultAction)
	m.Rules = firewallRulesToMap(m.Rules, fw.Rules, diags)
	m.Revision = types.Int64Value(int64(fw.Revision))

	findingType := types.ObjectType{AttrTypes: firewallFindingAttrTypes()}
	findings := make([]attr.Value, 0, len(fw.Findings))
	for _, f := range fw.Findings {
		// Only the answer to a write carries it, so keeping it would have
		// the next read plan a change nobody made.
		if f.Code == findingDeniesCaller {
			continue
		}
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

// attachedFirewall reads a target's firewall onto its two attributes:
// `firewall`, the reference the configuration wrote, and `firewall_id`, the
// firewall that reference resolved to. Nothing attached is null in both, and
// a firewall still being taken off the edge has no id and reads as none,
// which is what was asked for. The written reference is kept while it still
// names the attached firewall, by the id it resolved to, by its id or by its
// name; anything else attached is drift, and `firewall` then holds its id.
func attachedFirewall(ref *client.FirewallRef, written, resolved types.String) (firewall, firewallID types.String) {
	if ref == nil || ref.ID == "" {
		return types.StringNull(), types.StringNull()
	}
	id := types.StringValue(ref.ID)
	if written.IsNull() || written.IsUnknown() || written.ValueString() == "" {
		return id, id
	}
	if resolved.ValueString() == ref.ID || written.ValueString() == ref.ID || written.ValueString() == ref.Name {
		return written, id
	}
	return id, id
}

// firewallTarget is what an update does to a target's firewall, from the
// planned and the stored reference and the ids they resolved to. A respelled
// reference to the firewall already attached does nothing.
func firewallTarget(plan, planID, state, stateID types.String) (attach, detach bool) {
	if plan.IsNull() || plan.ValueString() == "" {
		return false, !state.IsNull() && state.ValueString() != ""
	}
	if plan.Equal(state) {
		return false, false
	}
	if !planID.IsUnknown() && !planID.IsNull() && planID.Equal(stateID) {
		return false, false
	}
	return true, false
}

// answeredFirewallID is the id a wait for the edge keys on: the one the API
// answered an attach with, since the configuration may have named the
// firewall by its name. written is the fallback for an answer carrying none.
func answeredFirewallID(ref *client.FirewallRef, written string) string {
	if ref != nil && ref.ID != "" {
		return ref.ID
	}
	return written
}

// resolveFirewall finds the firewall ref names in a project, by id or by name,
// the way the API resolves the firewall an attach names: an id in the
// project, else a name in it. One list read answers both.
func resolveFirewall(ctx context.Context, c *client.Client, projectID, ref string) (string, error) {
	firewalls, err := c.ListFirewalls(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, fw := range firewalls {
		if fw.ID == ref {
			return fw.ID, nil
		}
	}
	for _, fw := range firewalls {
		if fw.Name == ref {
			return fw.ID, nil
		}
	}
	return "", fmt.Errorf("there is no firewall %q in project %s", ref, projectID)
}

// planFirewallReference plans `firewall_id` for a target whose `firewall` is
// a reference, by name or id, to a firewall in the target's project. An
// unchanged reference keeps the id it resolved to and costs no request; a
// changed one is resolved once, so a respelling of the firewall already
// attached plans no attach; a reference that resolves to nothing fails the
// plan rather than the apply.
func planFirewallReference(ctx context.Context, c *client.Client, projectID string, config, state, stateID types.String, diags *diag.Diagnostics) types.String {
	switch {
	case config.IsUnknown():
		return types.StringUnknown()
	case config.IsNull() || config.ValueString() == "":
		return types.StringNull()
	case config.Equal(state):
		// State written before firewall_id existed has none to carry over;
		// the apply reads it.
		if stateID.IsNull() {
			return types.StringUnknown()
		}
		return stateID
	}
	id, err := resolveFirewall(ctx, c, projectID, config.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("firewall"), "Error resolving firewall",
			fmt.Sprintf("%q could not be resolved to a firewall, so the plan cannot tell another spelling of "+
				"the attached firewall from a different one: %s", config.ValueString(), err))
		return types.StringUnknown()
	}
	return types.StringValue(id)
}
