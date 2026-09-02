package executionenv

import (
	"fmt"
	"sort"
	"strings"
)

type RebuildPlan struct {
	EnvironmentKey string
	AdapterKey string
	Application string
	ApplicationVersionConstraint string
	PortableItems []string
	ReferenceItems []string
	ManualItems []string
	UnsupportedItems []string
	LaunchInstruction string
	RequiresDestinationInspection bool
}

func BuildRebuildPlan(manifest Manifest, snapshot *Snapshot) (RebuildPlan, error) {
	normalizedManifest, err := Normalize(manifest)
	if err != nil { return RebuildPlan{}, err }
	manifestHash, err := ContentHash(normalizedManifest)
	if err != nil { return RebuildPlan{}, err }
	plan := RebuildPlan{
		EnvironmentKey: normalizedManifest.EnvironmentKey,
		AdapterKey: normalizedManifest.AdapterKey,
		Application: normalizedManifest.Application.Name,
		ApplicationVersionConstraint: normalizedManifest.Application.VersionConstraint,
		RequiresDestinationInspection: true,
	}
	if normalizedManifest.Launch != nil {
		switch normalizedManifest.Launch.Kind {
		case LaunchAsset:
			plan.LaunchInstruction = "Open declared launch asset: " + normalizedManifest.Launch.AssetKey
		case LaunchLocator:
			plan.LaunchInstruction = "Open declared launch locator: " + normalizedManifest.Launch.Locator
		}
	}
	for _, asset := range normalizedManifest.Assets {
		label := asset.Name + " (manifest asset " + asset.Key + ")"
		if asset.CapturePolicy == CaptureContentBound {
			plan.PortableItems = append(plan.PortableItems, label)
		} else {
			plan.ReferenceItems = append(plan.ReferenceItems, label+": "+asset.Locator)
		}
	}
	for _, extension := range normalizedManifest.Extensions {
		plan.ManualItems = append(plan.ManualItems, fmt.Sprintf("Install/verify %s %s", extension.Name, extension.VersionConstraint))
	}
	for _, binding := range normalizedManifest.Bindings {
		plan.ManualItems = append(plan.ManualItems, "Verify binding: "+binding.Name)
	}
	if snapshot != nil {
		normalizedSnapshot, err := NormalizeSnapshot(*snapshot)
		if err != nil { return RebuildPlan{}, err }
		if normalizedSnapshot.EnvironmentKey != normalizedManifest.EnvironmentKey || normalizedSnapshot.AdapterKey != normalizedManifest.AdapterKey || normalizedSnapshot.SourceManifestSHA256 != manifestHash {
			return RebuildPlan{}, fmt.Errorf("snapshot does not belong to the supplied manifest identity")
		}
		if normalizedSnapshot.CaptureStatus == SnapshotUnsupported {
			plan.UnsupportedItems = append(plan.UnsupportedItems, "Adapter reported execution-environment snapshot capture unsupported")
		}
		for _, item := range normalizedSnapshot.Items {
			label := item.Name + " (snapshot item " + item.Key + ")"
			switch item.Capture {
			case ItemUnsupported:
				plan.UnsupportedItems = append(plan.UnsupportedItems, label)
			case ItemMissing:
				plan.ManualItems = append(plan.ManualItems, "Recreate or locate "+label)
			default:
				switch item.Portability {
				case SnapshotContentBound:
					plan.PortableItems = append(plan.PortableItems, label)
				case SnapshotReferenceOnly:
					plan.ReferenceItems = append(plan.ReferenceItems, label+": "+item.Locator)
				case SnapshotDescriptiveOnly:
					plan.ManualItems = append(plan.ManualItems, "Use recorded guidance for "+label)
				}
			}
		}
	}
	sort.Strings(plan.PortableItems)
	sort.Strings(plan.ReferenceItems)
	sort.Strings(plan.ManualItems)
	sort.Strings(plan.UnsupportedItems)
	return plan, nil
}

func RenderRebuildPlan(plan RebuildPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Execution Environment Rebuild Plan\nEnvironment: %s\nAdapter: %s\nApplication: %s (%s)\n", plan.EnvironmentKey, plan.AdapterKey, plan.Application, plan.ApplicationVersionConstraint)
	writePlanSection := func(title string, items []string) {
		b.WriteString("\n" + title + ":\n")
		if len(items) == 0 { b.WriteString("- none\n"); return }
		for _, item := range items { b.WriteString("- " + item + "\n") }
	}
	writePlanSection("Portable content", plan.PortableItems)
	writePlanSection("Reference-only dependencies", plan.ReferenceItems)
	writePlanSection("Manual reconstruction", plan.ManualItems)
	writePlanSection("Unsupported capture surfaces", plan.UnsupportedItems)
	b.WriteString("\nLaunch/open target:\n")
	if plan.LaunchInstruction == "" { b.WriteString("- none declared\n") } else { b.WriteString("- " + plan.LaunchInstruction + "\n") }
	if plan.RequiresDestinationInspection {
		b.WriteString("\nReadiness notice:\n- This plan is reconstruction guidance only. Destination readiness requires a fresh StageCore execution-environment inspection.\n")
	}
	return b.String()
}
