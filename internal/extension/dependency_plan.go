package extension

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type InstallPlanStatus string

const (
	InstallPlanReady                InstallPlanStatus = "READY"
	InstallPlanRequiresDependencies InstallPlanStatus = "REQUIRES_DEPENDENCIES"
	InstallPlanBlocked              InstallPlanStatus = "BLOCKED"
)

var (
	ErrDependenciesRequired = errors.New("required extension dependencies must be installed first")
	ErrDependencyPlanBlocked = errors.New("extension dependency plan is blocked")
)

type InstallPlanStep struct {
	Order       int    `json:"order"`
	PackageID   string `json:"package_id"`
	ExtensionID string `json:"extension_id"`
	Version     string `json:"version"`
	Kind        Kind   `json:"kind"`
	Source      Source `json:"source"`
}

type InstallPlanBlocker struct {
	Code             string `json:"code"`
	ExtensionID      string `json:"extension_id,omitempty"`
	RequiredBy       string `json:"required_by,omitempty"`
	MinVersion       string `json:"min_version,omitempty"`
	MaxVersion       string `json:"max_version,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Detail           string `json:"detail,omitempty"`
}

type InstallPlanWarning struct {
	Code             string `json:"code"`
	ExtensionID      string `json:"extension_id"`
	RequiredBy       string `json:"required_by"`
	MinVersion       string `json:"min_version,omitempty"`
	MaxVersion       string `json:"max_version,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	CandidateVersion string `json:"candidate_version,omitempty"`
}

type InstallPlan struct {
	RootPackageID        string               `json:"root_package_id"`
	RootExtensionID      string               `json:"root_extension_id"`
	RootVersion          string               `json:"root_version"`
	RootAlreadyInstalled bool                 `json:"root_already_installed"`
	Status               InstallPlanStatus    `json:"status"`
	Steps                []InstallPlanStep    `json:"steps"`
	Blockers             []InstallPlanBlocker `json:"blockers"`
	Warnings             []InstallPlanWarning `json:"warnings"`
}

type InstallPlanError struct {
	Cause error
	Plan  InstallPlan
}

func (e *InstallPlanError) Error() string {
	if e == nil || e.Cause == nil {
		return "extension install plan is not ready"
	}
	return e.Cause.Error()
}

func (e *InstallPlanError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type dependencyRequirement struct {
	RequiredBy string
	MinVersion string
	MaxVersion string
}

type dependencySolveState struct {
	selected  map[string]Package
	installed map[string]bool
	expanded  map[string]bool
	constraints map[string][]dependencyRequirement
	edges     map[string][]string
}

func newDependencySolveState() dependencySolveState {
	return dependencySolveState{
		selected: make(map[string]Package),
		installed: make(map[string]bool),
		expanded: make(map[string]bool),
		constraints: make(map[string][]dependencyRequirement),
		edges: make(map[string][]string),
	}
}

func (s dependencySolveState) clone() dependencySolveState {
	clone := newDependencySolveState()
	for key, value := range s.selected {
		clone.selected[key] = value
	}
	for key, value := range s.installed {
		clone.installed[key] = value
	}
	for key, value := range s.expanded {
		clone.expanded[key] = value
	}
	for key, values := range s.constraints {
		clone.constraints[key] = append([]dependencyRequirement(nil), values...)
	}
	for key, values := range s.edges {
		clone.edges[key] = append([]string(nil), values...)
	}
	return clone
}

type installedDependency struct {
	pkg   Package
	found bool
}

type dependencySolver struct {
	installer      *Installer
	installedCache map[string]installedDependency
	candidateCache map[string][]Package
}

func (i *Installer) PlanInstall(ctx context.Context, packageID string) (InstallPlan, error) {
	if i == nil || i.library == nil {
		return InstallPlan{}, fmt.Errorf("extension installer is unavailable")
	}
	packageID = strings.TrimSpace(packageID)
	if packageID == "" {
		return InstallPlan{}, fmt.Errorf("package ID is required")
	}
	root, err := i.library.Get(ctx, packageID)
	if err != nil {
		return InstallPlan{}, err
	}
	plan := InstallPlan{
		RootPackageID: root.PackageID,
		RootExtensionID: root.Manifest.ExtensionID,
		RootVersion: root.Manifest.Version,
		Status: InstallPlanBlocked,
		Steps: []InstallPlanStep{},
		Blockers: []InstallPlanBlocker{},
		Warnings: []InstallPlanWarning{},
	}
	if !root.Compatible {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{
			Code: "ROOT_PACKAGE_INCOMPATIBLE",
			ExtensionID: root.Manifest.ExtensionID,
			Detail: root.CompatibilityReason,
		})
		return plan, nil
	}

	solver := &dependencySolver{
		installer: i,
		installedCache: make(map[string]installedDependency),
		candidateCache: make(map[string][]Package),
	}
	installedRoot, found, err := solver.installedPackage(ctx, root.Manifest.ExtensionID)
	if err != nil {
		return InstallPlan{}, err
	}
	if found {
		if installedRoot.PackageID != root.PackageID {
			plan.Blockers = append(plan.Blockers, InstallPlanBlocker{
				Code: "ROOT_VERSION_ALREADY_INSTALLED",
				ExtensionID: root.Manifest.ExtensionID,
				InstalledVersion: installedRoot.Manifest.Version,
				Detail: "update or rollback is required before installing a different root package version",
			})
			return plan, nil
		}
		plan.RootAlreadyInstalled = true
	}

	state := newDependencySolveState()
	state.selected[root.Manifest.ExtensionID] = root
	state.installed[root.Manifest.ExtensionID] = plan.RootAlreadyInstalled
	solved, blocker, err := solver.solve(ctx, state)
	if err != nil {
		return InstallPlan{}, err
	}
	if blocker != nil {
		plan.Blockers = append(plan.Blockers, *blocker)
		return plan, nil
	}
	if cycle := detectDependencyCycle(root.Manifest.ExtensionID, solved.edges); len(cycle) != 0 {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{
			Code: "DEPENDENCY_CYCLE",
			ExtensionID: cycle[0],
			Detail: strings.Join(cycle, " -> "),
		})
		return plan, nil
	}

	order := dependencyInstallOrder(root.Manifest.ExtensionID, solved.edges)
	for _, extensionID := range order {
		if solved.installed[extensionID] {
			continue
		}
		pkg := solved.selected[extensionID]
		plan.Steps = append(plan.Steps, InstallPlanStep{
			Order: len(plan.Steps) + 1,
			PackageID: pkg.PackageID,
			ExtensionID: pkg.Manifest.ExtensionID,
			Version: pkg.Manifest.Version,
			Kind: pkg.Manifest.Kind,
			Source: pkg.Manifest.Source,
		})
	}
	warnings, err := solver.optionalWarnings(ctx, solved)
	if err != nil {
		return InstallPlan{}, err
	}
	plan.Warnings = warnings
	plan.Status = InstallPlanReady
	for _, step := range plan.Steps {
		if step.PackageID != root.PackageID {
			plan.Status = InstallPlanRequiresDependencies
			break
		}
	}
	return plan, nil
}

func (s *dependencySolver) solve(ctx context.Context, state dependencySolveState) (dependencySolveState, *InstallPlanBlocker, error) {
	for {
		if extensionID := nextUnexpandedSelection(state); extensionID != "" {
			pkg := state.selected[extensionID]
			for _, dependency := range pkg.Manifest.Dependencies {
				if dependency.Optional {
					continue
				}
				state.constraints[dependency.ExtensionID] = appendRequirement(
					state.constraints[dependency.ExtensionID],
					dependencyRequirement{RequiredBy: extensionID, MinVersion: dependency.MinVersion, MaxVersion: dependency.MaxVersion},
				)
				state.edges[extensionID] = appendUniqueString(state.edges[extensionID], dependency.ExtensionID)
				if selected, exists := state.selected[dependency.ExtensionID]; exists && !packageSatisfiesRequirements(selected, state.constraints[dependency.ExtensionID]) {
					blocker := selectedConflictBlocker(dependency.ExtensionID, state.constraints[dependency.ExtensionID], selected.Manifest.Version, state.installed[dependency.ExtensionID])
					return dependencySolveState{}, &blocker, nil
				}
			}
			state.expanded[extensionID] = true
			continue
		}

		unresolved := nextUnresolvedDependency(state)
		if unresolved == "" {
			return state, nil, nil
		}
		requirements := state.constraints[unresolved]
		if minVersion, maxVersion, impossible := effectiveRequirementRange(requirements); impossible {
			blocker := InstallPlanBlocker{
				Code: "DEPENDENCY_CONSTRAINT_CONFLICT",
				ExtensionID: unresolved,
				MinVersion: minVersion,
				MaxVersion: maxVersion,
				Detail: requirementDetail(requirements),
			}
			return dependencySolveState{}, &blocker, nil
		}

		installed, found, err := s.installedPackage(ctx, unresolved)
		if err != nil {
			return dependencySolveState{}, nil, err
		}
		if found {
			if !packageSatisfiesRequirements(installed, requirements) {
				blocker := selectedConflictBlocker(unresolved, requirements, installed.Manifest.Version, true)
				return dependencySolveState{}, &blocker, nil
			}
			state.selected[unresolved] = installed
			state.installed[unresolved] = true
			continue
		}

		candidates, err := s.candidates(ctx, unresolved)
		if err != nil {
			return dependencySolveState{}, nil, err
		}
		matching := make([]Package, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Compatible && packageSatisfiesRequirements(candidate, requirements) {
				matching = append(matching, candidate)
			}
		}
		if len(matching) == 0 {
			minVersion, maxVersion, _ := effectiveRequirementRange(requirements)
			code := "DEPENDENCY_NO_COMPATIBLE_CANDIDATE"
			if len(candidates) == 0 {
				code = "DEPENDENCY_UNAVAILABLE"
			}
			blocker := InstallPlanBlocker{
				Code: code,
				ExtensionID: unresolved,
				RequiredBy: primaryRequiredBy(requirements),
				MinVersion: minVersion,
				MaxVersion: maxVersion,
				Detail: requirementDetail(requirements),
			}
			return dependencySolveState{}, &blocker, nil
		}

		var firstBlocker *InstallPlanBlocker
		for _, candidate := range matching {
			trial := state.clone()
			trial.selected[unresolved] = candidate
			trial.installed[unresolved] = false
			solved, blocker, err := s.solve(ctx, trial)
			if err != nil {
				return dependencySolveState{}, nil, err
			}
			if blocker == nil {
				return solved, nil, nil
			}
			if firstBlocker == nil {
				copy := *blocker
				firstBlocker = &copy
			}
		}
		return dependencySolveState{}, firstBlocker, nil
	}
}

func (s *dependencySolver) installedPackage(ctx context.Context, extensionID string) (Package, bool, error) {
	if cached, ok := s.installedCache[extensionID]; ok {
		return cached.pkg, cached.found, nil
	}
	installations, err := s.installer.List(ctx, extensionID)
	if err != nil {
		return Package{}, false, err
	}
	if len(installations) == 0 {
		s.installedCache[extensionID] = installedDependency{}
		return Package{}, false, nil
	}
	if len(installations) != 1 {
		return Package{}, false, fmt.Errorf("extension %s has multiple installed package records", extensionID)
	}
	pkg, err := s.installer.library.Get(ctx, installations[0].PackageID)
	if err != nil {
		return Package{}, false, err
	}
	s.installedCache[extensionID] = installedDependency{pkg: pkg, found: true}
	return pkg, true, nil
}

func (s *dependencySolver) candidates(ctx context.Context, extensionID string) ([]Package, error) {
	if cached, ok := s.candidateCache[extensionID]; ok {
		return append([]Package(nil), cached...), nil
	}
	items, err := s.installer.library.List(ctx, extensionID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(left, right int) bool {
		comparison := compareSemanticVersions(items[left].Manifest.Version, items[right].Manifest.Version)
		if comparison != 0 {
			return comparison > 0
		}
		if items[left].ProductionReady != items[right].ProductionReady {
			return items[left].ProductionReady
		}
		if sourceRank(items[left].Manifest.Source) != sourceRank(items[right].Manifest.Source) {
			return sourceRank(items[left].Manifest.Source) > sourceRank(items[right].Manifest.Source)
		}
		return items[left].PackageID < items[right].PackageID
	})
	s.candidateCache[extensionID] = append([]Package(nil), items...)
	return items, nil
}

func (s *dependencySolver) optionalWarnings(ctx context.Context, state dependencySolveState) ([]InstallPlanWarning, error) {
	exts := make([]string, 0, len(state.selected))
	for extensionID := range state.selected {
		exts = append(exts, extensionID)
	}
	sort.Strings(exts)
	warnings := make([]InstallPlanWarning, 0)
	for _, extensionID := range exts {
		pkg := state.selected[extensionID]
		for _, dependency := range pkg.Manifest.Dependencies {
			if !dependency.Optional {
				continue
			}
			warning := InstallPlanWarning{
				ExtensionID: dependency.ExtensionID,
				RequiredBy: extensionID,
				MinVersion: dependency.MinVersion,
				MaxVersion: dependency.MaxVersion,
			}
			installed, found, err := s.installedPackage(ctx, dependency.ExtensionID)
			if err != nil {
				return nil, err
			}
			if found {
				if versionInRange(installed.Manifest.Version, dependency.MinVersion, dependency.MaxVersion) {
					continue
				}
				warning.Code = "OPTIONAL_DEPENDENCY_VERSION_MISMATCH"
				warning.InstalledVersion = installed.Manifest.Version
				warnings = append(warnings, warning)
				continue
			}
			candidates, err := s.candidates(ctx, dependency.ExtensionID)
			if err != nil {
				return nil, err
			}
			for _, candidate := range candidates {
				if candidate.Compatible && versionInRange(candidate.Manifest.Version, dependency.MinVersion, dependency.MaxVersion) {
					warning.Code = "OPTIONAL_DEPENDENCY_AVAILABLE"
					warning.CandidateVersion = candidate.Manifest.Version
					break
				}
			}
			if warning.Code == "" {
				warning.Code = "OPTIONAL_DEPENDENCY_UNAVAILABLE"
			}
			warnings = append(warnings, warning)
		}
	}
	return warnings, nil
}

func nextUnexpandedSelection(state dependencySolveState) string {
	items := make([]string, 0)
	for extensionID := range state.selected {
		if !state.expanded[extensionID] {
			items = append(items, extensionID)
		}
	}
	sort.Strings(items)
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func nextUnresolvedDependency(state dependencySolveState) string {
	items := make([]string, 0)
	for extensionID := range state.constraints {
		if _, selected := state.selected[extensionID]; !selected {
			items = append(items, extensionID)
		}
	}
	sort.Strings(items)
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func appendRequirement(values []dependencyRequirement, requirement dependencyRequirement) []dependencyRequirement {
	for _, existing := range values {
		if existing == requirement {
			return values
		}
	}
	return append(values, requirement)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func packageSatisfiesRequirements(pkg Package, requirements []dependencyRequirement) bool {
	for _, requirement := range requirements {
		if !versionInRange(pkg.Manifest.Version, requirement.MinVersion, requirement.MaxVersion) {
			return false
		}
	}
	return true
}

func effectiveRequirementRange(requirements []dependencyRequirement) (string, string, bool) {
	minVersion := ""
	maxVersion := ""
	for _, requirement := range requirements {
		if requirement.MinVersion != "" && (minVersion == "" || compareSemanticVersions(requirement.MinVersion, minVersion) > 0) {
			minVersion = requirement.MinVersion
		}
		if requirement.MaxVersion != "" && (maxVersion == "" || compareSemanticVersions(requirement.MaxVersion, maxVersion) < 0) {
			maxVersion = requirement.MaxVersion
		}
	}
	return minVersion, maxVersion, minVersion != "" && maxVersion != "" && compareSemanticVersions(minVersion, maxVersion) > 0
}

func primaryRequiredBy(requirements []dependencyRequirement) string {
	if len(requirements) == 0 {
		return ""
	}
	items := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		items = append(items, requirement.RequiredBy)
	}
	sort.Strings(items)
	return items[0]
}

func requirementDetail(requirements []dependencyRequirement) string {
	items := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		rangeText := "any version"
		switch {
		case requirement.MinVersion != "" && requirement.MaxVersion != "":
			rangeText = requirement.MinVersion + ".." + requirement.MaxVersion
		case requirement.MinVersion != "":
			rangeText = ">=" + requirement.MinVersion
		case requirement.MaxVersion != "":
			rangeText = "<=" + requirement.MaxVersion
		}
		items = append(items, requirement.RequiredBy+":"+rangeText)
	}
	sort.Strings(items)
	return strings.Join(items, ", ")
}

func selectedConflictBlocker(extensionID string, requirements []dependencyRequirement, installedVersion string, installed bool) InstallPlanBlocker {
	minVersion, maxVersion, _ := effectiveRequirementRange(requirements)
	code := "DEPENDENCY_SELECTED_VERSION_CONFLICT"
	if installed {
		code = "DEPENDENCY_INSTALLED_VERSION_CONFLICT"
	}
	return InstallPlanBlocker{
		Code: code,
		ExtensionID: extensionID,
		RequiredBy: primaryRequiredBy(requirements),
		MinVersion: minVersion,
		MaxVersion: maxVersion,
		InstalledVersion: installedVersion,
		Detail: requirementDetail(requirements),
	}
}

func sourceRank(source Source) int {
	switch source {
	case SourceOfficial:
		return 3
	case SourceLocal:
		return 2
	case SourceCommunity:
		return 1
	default:
		return 0
	}
}

func detectDependencyCycle(root string, edges map[string][]string) []string {
	visited := make(map[string]bool)
	active := make(map[string]int)
	stack := make([]string, 0)
	var visit func(string) []string
	visit = func(node string) []string {
		if index, ok := active[node]; ok {
			cycle := append([]string(nil), stack[index:]...)
			cycle = append(cycle, node)
			return cycle
		}
		if visited[node] {
			return nil
		}
		active[node] = len(stack)
		stack = append(stack, node)
		children := append([]string(nil), edges[node]...)
		sort.Strings(children)
		for _, child := range children {
			if cycle := visit(child); len(cycle) != 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		delete(active, node)
		visited[node] = true
		return nil
	}
	return visit(root)
}

func dependencyInstallOrder(root string, edges map[string][]string) []string {
	visited := make(map[string]bool)
	order := make([]string, 0)
	var visit func(string)
	visit = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		children := append([]string(nil), edges[node]...)
		sort.Strings(children)
		for _, child := range children {
			visit(child)
		}
		order = append(order, node)
	}
	visit(root)
	return order
}
