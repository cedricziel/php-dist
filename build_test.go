package phpdist_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/chronos"

	//nolint Ignore SA1019, informed usage of deprecated package
	"github.com/paketo-buildpacks/packit/v2/paketosbom"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	phpdist "github.com/paketo-buildpacks/php-dist"
	"github.com/paketo-buildpacks/php-dist/fakes"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
	. "github.com/paketo-buildpacks/occam/matchers"
)

func testBuild(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		layersDir         string
		workingDir        string
		cnbDir            string
		entryResolver     *fakes.EntryResolver
		dependencyManager *fakes.DependencyManager
		files             *fakes.FileManager
		clock             chronos.Clock
		environment       *fakes.EnvironmentConfiguration
		buffer            *bytes.Buffer

		build packit.BuildFunc
	)

	it.Before(func() {
		var err error
		layersDir, err = os.MkdirTemp("", "layers")
		Expect(err).NotTo(HaveOccurred())

		cnbDir, err = os.MkdirTemp("", "cnb")
		Expect(err).NotTo(HaveOccurred())

		entryResolver = &fakes.EntryResolver{}
		entryResolver.ResolveCall.Returns.BuildpackPlanEntry = packit.BuildpackPlanEntry{
			Name: "php",
			Metadata: map[string]interface{}{
				"version":        "7.2.*",
				"version-source": "some-source",
			},
		}

		entryResolver.MergeLayerTypesCall.Returns.Launch = false
		entryResolver.MergeLayerTypesCall.Returns.Build = false

		dependencyManager = &fakes.DependencyManager{}
		dependencyManager.ResolveCall.Returns.Dependency = postal.Dependency{Name: "PHP"}
		dependencyManager.GenerateBillOfMaterialsCall.Returns.BOMEntrySlice = []packit.BOMEntry{
			{
				Name: "php",
				Metadata: paketosbom.BOMMetadata{
					Version: "php-dependency-version",
					Checksum: paketosbom.BOMChecksum{
						Algorithm: paketosbom.SHA256,
						Hash:      "php-dependency-sha",
					},
					URI: "php-dependency-uri",
				},
			},
		}

		files = &fakes.FileManager{}
		files.FindExtensionsCall.Returns.String = "no-debug-non-zts-12345"
		files.WriteConfigCall.Returns.DefaultConfig = "some/ini/path/php.ini"
		files.WriteConfigCall.Returns.BuildpackConfig = "some/other/path/buildpack.ini"

		clock = chronos.DefaultClock

		environment = &fakes.EnvironmentConfiguration{}

		workingDir, err = os.MkdirTemp("", "working-dir")
		Expect(err).NotTo(HaveOccurred())

		buffer = bytes.NewBuffer(nil)
		logEmitter := scribe.NewEmitter(buffer)

		build = phpdist.Build(entryResolver, dependencyManager, files, environment, logEmitter, clock)
	})

	it.After(func() {
		Expect(os.RemoveAll(layersDir)).To(Succeed())
		Expect(os.RemoveAll(cnbDir)).To(Succeed())
		Expect(os.RemoveAll(workingDir)).To(Succeed())
	})

	it("returns a result that builds correctly", func() {
		result, err := build(packit.BuildContext{
			WorkingDir: workingDir,
			CNBPath:    cnbDir,
			Stack:      "some-stack",
			BuildpackInfo: packit.BuildpackInfo{
				Name:    "Some Buildpack",
				Version: "some-version",
			},
			Plan: packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "php",
						Metadata: map[string]interface{}{
							"version":        "7.2.*",
							"version-source": "some-source",
						},
					},
				},
			},
			Platform: packit.Platform{Path: "some-platform-dir"},
			Layers:   packit.Layers{Path: layersDir},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(len(result.Layers)).To(Equal(1))
		Expect(result.Layers[0].Name).To(Equal("php"))
		Expect(result.Layers[0].Path).To(Equal(filepath.Join(layersDir, "php")))
		Expect(result.Layers[0].Metadata[phpdist.DepKey]).To(Equal(""))

		Expect(filepath.Join(layersDir, "php")).To(BeADirectory())

		Expect(entryResolver.ResolveCall.Receives.Name).To(Equal(phpdist.PHPDependency))
		Expect(entryResolver.ResolveCall.Receives.Entries).To(Equal([]packit.BuildpackPlanEntry{
			{
				Name: "php",
				Metadata: map[string]interface{}{
					"version":        "7.2.*",
					"version-source": "some-source",
				},
			},
		}))
		Expect(entryResolver.ResolveCall.Receives.Priorities).To(Equal([]interface{}{
			"BP_PHP_VERSION",
			"buildpack.yml",
			"composer.lock",
			"composer.json",
			"default-versions",
		}))

		Expect(dependencyManager.ResolveCall.Receives.Path).To(Equal(filepath.Join(cnbDir, "buildpack.toml")))
		Expect(dependencyManager.ResolveCall.Receives.Id).To(Equal("php"))
		Expect(dependencyManager.ResolveCall.Receives.Version).To(Equal("7.2.*"))
		Expect(dependencyManager.ResolveCall.Receives.Stack).To(Equal("some-stack"))

		Expect(dependencyManager.DeliverCall.Receives.Dependency).To(Equal(postal.Dependency{Name: "PHP"}))
		Expect(dependencyManager.DeliverCall.Receives.CnbPath).To(Equal(cnbDir))
		Expect(dependencyManager.DeliverCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "php")))
		Expect(dependencyManager.DeliverCall.Receives.PlatformPath).To(Equal("some-platform-dir"))

		Expect(files.FindExtensionsCall.Receives.LayerRoot).To(Equal(filepath.Join(layersDir, "php")))

		Expect(files.WriteConfigCall.Receives.LayerRoot).To(Equal(filepath.Join(layersDir, "php")))
		Expect(files.WriteConfigCall.Receives.CnbPath).To(Equal(cnbDir))
		Expect(files.WriteConfigCall.Receives.Data).To(Equal(phpdist.PhpIniConfig{
			IncludePath: strings.Join([]string{
				filepath.Join(layersDir, "php", "lib", "php"),
				filepath.Join(workingDir, "lib"),
			}, ":"),
			ExtensionDir: "no-debug-non-zts-12345",
		}))

		Expect(environment.ConfigureCall.CallCount).To(Equal(1))
		Expect(environment.ConfigureCall.Receives.Layer.Path).To(Equal(filepath.Join(layersDir, "php")))
		Expect(environment.ConfigureCall.Receives.ExtensionsDir).To(Equal("no-debug-non-zts-12345"))
		Expect(environment.ConfigureCall.Receives.DefaultIni).To(Equal("some/ini/path/php.ini"))
		Expect(environment.ConfigureCall.Receives.ScanDirs).To(Equal([]string{
			"some/ini/path",
			"some/other/path",
			filepath.Join(workingDir, "php.ini.d"),
		}))

		Expect(buffer.String()).To(ContainSubstring("Some Buildpack some-version"))
		Expect(buffer.String()).To(ContainSubstring("Resolving PHP version"))
		Expect(buffer.String()).To(ContainSubstring("Selected PHP version (using some-source): "))
		Expect(buffer.String()).To(ContainSubstring("Executing build process"))
	})

	context("when the $BP_PHP_LIB_DIR is set", func() {
		it.Before(func() {
			Expect(os.Setenv("BP_PHP_LIB_DIR", "user-lib-dir")).To(Succeed())
		})
		it.After(func() {
			Expect(os.Unsetenv("BP_PHP_LIB_DIR")).To(Succeed())
		})

		it("the config file lib path contains the $BP_PHP_LIB_DIR instead of the default", func() {
			_, err := build(packit.BuildContext{
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				WorkingDir: workingDir,
				Plan: packit.BuildpackPlan{
					Entries: []packit.BuildpackPlanEntry{
						{
							Name: "php",
							Metadata: map[string]interface{}{
								"version":        "7.2.*",
								"version-source": "some-source",
								"launch":         true,
								"build":          true,
							},
						},
					},
				},
				Layers: packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(files.WriteConfigCall.Receives.Data).To(Equal(phpdist.PhpIniConfig{
				IncludePath: strings.Join([]string{
					filepath.Join(layersDir, "php", "lib", "php"),
					filepath.Join(workingDir, "user-lib-dir"),
				}, ":"),
				ExtensionDir: "no-debug-non-zts-12345",
			}))
		})
	})

	context("when the build plan entry includes the build, launch flags", func() {
		var workingDir string

		it.Before(func() {
			var err error
			workingDir, err = os.MkdirTemp("", "working-dir")
			Expect(err).NotTo(HaveOccurred())

			entryResolver.ResolveCall.Returns.BuildpackPlanEntry = packit.BuildpackPlanEntry{
				Name: "php",
				Metadata: map[string]interface{}{
					"version":        "7.2.*",
					"version-source": "some-source",
					"launch":         true,
					"build":          true,
				},
			}
			entryResolver.MergeLayerTypesCall.Returns.Launch = true
			entryResolver.MergeLayerTypesCall.Returns.Build = true
		})

		it.After(func() {
			Expect(os.RemoveAll(workingDir)).To(Succeed())
		})

		it("marks the php layer as build, cache and launch", func() {
			result, err := build(packit.BuildContext{
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				WorkingDir: workingDir,
				Plan: packit.BuildpackPlan{
					Entries: []packit.BuildpackPlanEntry{
						{
							Name: "php",
							Metadata: map[string]interface{}{
								"version":        "7.2.*",
								"version-source": "some-source",
								"launch":         true,
								"build":          true,
							},
						},
					},
				},
				Layers: packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Layers)).To(Equal(1))
			Expect(result.Layers[0].Name).To(Equal("php"))
			Expect(result.Layers[0].Path).To(Equal(filepath.Join(layersDir, "php")))
			Expect(result.Layers[0].Metadata[phpdist.DepKey]).To(Equal(""))

			Expect(result.Layers[0].Build).To(BeTrue())
			Expect(result.Layers[0].Cache).To(BeTrue())
			Expect(result.Layers[0].Launch).To(BeTrue())

			Expect(result.Launch.BOM).To(Equal([]packit.BOMEntry{
				{
					Name: "php",
					Metadata: paketosbom.BOMMetadata{
						Version: "php-dependency-version",
						Checksum: paketosbom.BOMChecksum{
							Algorithm: paketosbom.SHA256,
							Hash:      "php-dependency-sha",
						},
						URI: "php-dependency-uri",
					},
				},
			}))
			Expect(result.Build.BOM).To(Equal([]packit.BOMEntry{
				{
					Name: "php",
					Metadata: paketosbom.BOMMetadata{
						Version: "php-dependency-version",
						Checksum: paketosbom.BOMChecksum{
							Algorithm: paketosbom.SHA256,
							Hash:      "php-dependency-sha",
						},
						URI: "php-dependency-uri",
					},
				},
			}))
		})
	})

	context("when the version source of the resolved Buildpack Plan entry is buildpack.yml", func() {
		it.Before(func() {
			entryResolver.ResolveCall.Returns.BuildpackPlanEntry = packit.BuildpackPlanEntry{
				Name: "php",
				Metadata: map[string]interface{}{
					"version":        "7.2.*",
					"version-source": "buildpack.yml",
				},
			}
		})

		it("logs a warning to switch to env var configuration", func() {
			_, err := build(packit.BuildContext{
				WorkingDir: workingDir,
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				BuildpackInfo: packit.BuildpackInfo{
					Name:    "Some PHP Buildpack",
					Version: "9.9.9",
				},
				Plan: packit.BuildpackPlan{
					Entries: []packit.BuildpackPlanEntry{},
				},
				Layers: packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(buffer).To(ContainLines(
				"    Selected PHP version (using buildpack.yml): ",
				"",
				"    WARNING: Setting the PHP version through buildpack.yml will be deprecated in Some PHP Buildpack v10.0.0.",
				"    In versions >= v10.0.0, use the $BP_PHP_VERSION environment variable to specify a version.",
			))
		})
	})

	context("when there is a dependency cache match", func() {
		it.Before(func() {
			err := os.WriteFile(filepath.Join(layersDir, "php.toml"), []byte("[metadata]\ndependency-sha = \"some-sha\"\n"), 0644)
			Expect(err).NotTo(HaveOccurred())

			dependencyManager.ResolveCall.Returns.Dependency = postal.Dependency{
				Name:   "PHP",
				SHA256: "some-sha",
			}
			entryResolver.MergeLayerTypesCall.Returns.Launch = true
			entryResolver.MergeLayerTypesCall.Returns.Build = true
		})

		it("exits build process early", func() {
			result, err := build(packit.BuildContext{
				CNBPath: cnbDir,
				Stack:   "some-stack",
				BuildpackInfo: packit.BuildpackInfo{
					Name:    "Some Buildpack",
					Version: "some-version",
				},
				Plan: packit.BuildpackPlan{
					Entries: []packit.BuildpackPlanEntry{
						{
							Name: "php",
							Metadata: map[string]interface{}{
								"version":        "7.2.*",
								"version-source": "some-source",
								"launch":         true,
								"build":          true,
							},
						},
					},
				},
				Layers: packit.Layers{Path: layersDir},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Launch.BOM).To(Equal(
				[]packit.BOMEntry{
					{
						Name: "php",
						Metadata: paketosbom.BOMMetadata{
							Version: "php-dependency-version",
							Checksum: paketosbom.BOMChecksum{
								Algorithm: paketosbom.SHA256,
								Hash:      "php-dependency-sha",
							},
							URI: "php-dependency-uri",
						},
					},
				},
			))

			Expect(result.Build.BOM).To(Equal(
				[]packit.BOMEntry{
					{
						Name: "php",
						Metadata: paketosbom.BOMMetadata{
							Version: "php-dependency-version",
							Checksum: paketosbom.BOMChecksum{
								Algorithm: paketosbom.SHA256,
								Hash:      "php-dependency-sha",
							},
							URI: "php-dependency-uri",
						},
					},
				},
			))

			Expect(dependencyManager.DeliverCall.CallCount).To(Equal(0))

			Expect(buffer.String()).To(ContainSubstring("Some Buildpack some-version"))
			Expect(buffer.String()).To(ContainSubstring("Resolving PHP version"))
			Expect(buffer.String()).To(ContainSubstring("Selected PHP version (using some-source): "))
			Expect(buffer.String()).To(ContainSubstring("Reusing cached layer"))
			Expect(buffer.String()).ToNot(ContainSubstring("Executing build process"))
		})
	})

	context("failure cases", func() {
		context("when a dependency cannot be resolved", func() {
			it.Before(func() {
				dependencyManager.ResolveCall.Returns.Error = errors.New("failed to resolve dependency")
			})

			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					CNBPath: cnbDir,
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "php",
								Metadata: map[string]interface{}{
									"version":        "7.2.*",
									"version-source": "some-source",
								},
							},
						},
					},
					Layers: packit.Layers{Path: layersDir},
				})
				Expect(err).To(MatchError("failed to resolve dependency"))
			})
		})

		context("when a dependency cannot be installed", func() {
			it.Before(func() {
				dependencyManager.DeliverCall.Returns.Error = errors.New("failed to install dependency")
			})

			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					CNBPath: cnbDir,
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "php",
								Metadata: map[string]interface{}{
									"version":        "7.2.*",
									"version-source": "some-source",
								},
							},
						},
					},
					Layers: packit.Layers{Path: layersDir},
				})
				Expect(err).To(MatchError("failed to install dependency"))
			})
		})

		context("when the layers directory cannot be written to", func() {
			it.Before(func() {
				Expect(os.Chmod(layersDir, 0000)).To(Succeed())
			})

			it.After(func() {
				Expect(os.Chmod(layersDir, os.ModePerm)).To(Succeed())
			})

			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					CNBPath: cnbDir,
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "php",
								Metadata: map[string]interface{}{
									"version":        "7.2.*",
									"version-source": "some-source",
								},
							},
						},
					},
					Layers: packit.Layers{Path: layersDir},
				})
				Expect(err).To(MatchError(ContainSubstring("permission denied")))
			})
		})
		context("NEW when finding PHP extensions fails", func() {
			it.Before(func() {
				files.FindExtensionsCall.Returns.Error = errors.New("cannot find extensions")
			})
			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					CNBPath: cnbDir,
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "php",
								Metadata: map[string]interface{}{
									"version":        "7.2.*",
									"version-source": "some-source",
								},
							},
						},
					},
					Layers: packit.Layers{Path: layersDir},
				})
				Expect(err).To(MatchError(ContainSubstring("cannot find extensions")))
			})
		})
		// TODO: Remove NEW from tests
		context("NEW when writing default php.ini fails", func() {
			it.Before(func() {
				files.WriteConfigCall.Returns.Err = errors.New("some config writing error")
			})
			it("returns an error", func() {
				_, err := build(packit.BuildContext{
					CNBPath: cnbDir,
					Plan: packit.BuildpackPlan{
						Entries: []packit.BuildpackPlanEntry{
							{
								Name: "php",
								Metadata: map[string]interface{}{
									"version":        "7.2.*",
									"version-source": "some-source",
								},
							},
						},
					},
					Layers: packit.Layers{Path: layersDir},
				})
				Expect(err).To(MatchError(ContainSubstring("some config writing error")))
			})
		})
	})
}
