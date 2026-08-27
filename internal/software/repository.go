package software

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

const CurrentHubAPIVersion = 1

type Repository struct {
	vault      *vault.Vault
	store      *store.Store
	apiVersion int
}

type ImportParams struct {
	ProductID          string
	Version            string
	Platform           string
	Architecture       string
	MinAPIVersion      int
	MaxAPIVersion      int
	OriginalFilename   string
	SigningStatus      string
	NotarizationStatus string
	ReleaseChannel     string
	ReleaseNotes       string
}

type PackageStatus struct {
	Package         store.SoftwarePackage
	Compatible      bool
	ProductionReady bool
	Preferred       bool
	Reason          string
}

func New(v *vault.Vault, s *store.Store, apiVersion int) (*Repository, error) {
	if v == nil || s == nil {
		return nil, fmt.Errorf("software repository requires Vault and store")
	}
	if apiVersion < 0 {
		return nil, fmt.Errorf("software repository API version cannot be negative")
	}
	return &Repository{vault: v, store: s, apiVersion: apiVersion}, nil
}

func (r *Repository) ImportPackage(ctx context.Context, p ImportParams, source io.Reader) (store.SoftwarePackage, error) {
	if r == nil || r.vault == nil || r.store == nil {
		return store.SoftwarePackage{}, fmt.Errorf("software repository is unavailable")
	}
	object, err := r.vault.ImportObject(ctx, source)
	if err != nil {
		return store.SoftwarePackage{}, fmt.Errorf("import software package object: %w", err)
	}
	pkg, err := r.store.RegisterSoftwarePackage(ctx, store.RegisterSoftwarePackageParams{
		ProductID: p.ProductID, Version: p.Version, Platform: p.Platform, Architecture: p.Architecture,
		MinAPIVersion: p.MinAPIVersion, MaxAPIVersion: p.MaxAPIVersion,
		ContentHash: object.ContentHash, SizeBytes: object.SizeBytes, OriginalFilename: p.OriginalFilename,
		SigningStatus: p.SigningStatus, NotarizationStatus: p.NotarizationStatus,
		ReleaseChannel: p.ReleaseChannel, ReleaseNotes: p.ReleaseNotes,
	})
	if err != nil {
		return store.SoftwarePackage{}, fmt.Errorf("register software package metadata: %w", err)
	}
	return pkg, nil
}

func (r *Repository) List(ctx context.Context, productID, platform, architecture string) ([]PackageStatus, error) {
	if r == nil || r.store == nil {
		return nil, fmt.Errorf("software repository is unavailable")
	}
	packages, err := r.store.ListSoftwarePackages(ctx, productID)
	if err != nil {
		return nil, err
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	architecture = strings.ToLower(strings.TrimSpace(architecture))
	statuses := make([]PackageStatus, 0, len(packages))
	for _, pkg := range packages {
		if platform != "" && pkg.Platform != platform {
			continue
		}
		if architecture != "" && pkg.Architecture != architecture {
			continue
		}
		statuses = append(statuses, r.status(pkg))
	}
	sort.SliceStable(statuses, func(i, j int) bool {
		if statuses[i].Compatible != statuses[j].Compatible {
			return statuses[i].Compatible
		}
		if statuses[i].ProductionReady != statuses[j].ProductionReady {
			return statuses[i].ProductionReady
		}
		if statuses[i].Package.ReleaseChannel != statuses[j].Package.ReleaseChannel {
			return statuses[i].Package.ReleaseChannel == store.SoftwareChannelRelease
		}
		return statuses[i].Package.CreatedAt.After(statuses[j].Package.CreatedAt)
	})
	for i := range statuses {
		if statuses[i].Compatible {
			statuses[i].Preferred = true
			break
		}
	}
	return statuses, nil
}

func (r *Repository) Get(ctx context.Context, packageID string) (PackageStatus, error) {
	if r == nil || r.store == nil {
		return PackageStatus{}, fmt.Errorf("software repository is unavailable")
	}
	pkg, err := r.store.GetSoftwarePackage(ctx, packageID)
	if err != nil {
		return PackageStatus{}, err
	}
	return r.status(pkg), nil
}

func (r *Repository) OpenPackage(ctx context.Context, packageID string) (*os.File, PackageStatus, error) {
	status, err := r.Get(ctx, packageID)
	if err != nil {
		return nil, PackageStatus{}, err
	}
	if !status.Compatible {
		return nil, status, fmt.Errorf("software package is incompatible with Hub API version %d", r.apiVersion)
	}
	file, object, err := r.vault.OpenObject(ctx, status.Package.ContentHash)
	if err != nil {
		return nil, status, err
	}
	if object.SizeBytes != status.Package.SizeBytes {
		_ = file.Close()
		return nil, status, fmt.Errorf("software package Vault metadata mismatch")
	}
	return file, status, nil
}

func (r *Repository) status(pkg store.SoftwarePackage) PackageStatus {
	compatible := r.apiVersion >= pkg.MinAPIVersion && r.apiVersion <= pkg.MaxAPIVersion
	productionReady := pkg.ReleaseChannel == store.SoftwareChannelRelease && pkg.SigningStatus == store.SoftwareSigningSigned
	if pkg.Platform == "macos" {
		productionReady = productionReady && pkg.NotarizationStatus == store.SoftwareNotarizationNotarized
	}
	reason := "compatible"
	if !compatible {
		reason = fmt.Sprintf("requires Hub API %d..%d; current is %d", pkg.MinAPIVersion, pkg.MaxAPIVersion, r.apiVersion)
	} else if !productionReady {
		if pkg.ReleaseChannel == store.SoftwareChannelDevelopment {
			reason = "compatible development build"
		} else {
			reason = "compatible but not production-ready"
		}
	}
	return PackageStatus{Package: pkg, Compatible: compatible, ProductionReady: productionReady, Reason: reason}
}
