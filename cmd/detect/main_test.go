/*
 * Copyright 2018-2019 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cloudfoundry/php-dist-cnb/php"

	"github.com/buildpack/libbuildpack/buildplan"
	"github.com/cloudfoundry/libcfbuildpack/helper"

	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/libcfbuildpack/detect"
	"github.com/cloudfoundry/libcfbuildpack/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestUnitDetect(t *testing.T) {
	spec.Run(t, "Detect", testDetect, spec.Report(report.Terminal{}))
}

func testDetect(t *testing.T, when spec.G, it spec.S) {
	var factory *test.DetectFactory

	it.Before(func() {
		RegisterTestingT(t)
		factory = test.NewDetectFactory(t)
	})

	it("always passes", func() {
		code, err := runDetect(factory.Detect)
		if err != nil {
			t.Error("Err in detect : ", err)
		}

		if diff := cmp.Diff(code, detect.PassStatusCode); diff != "" {
			t.Error("Problem : ", diff)
		}
	})

	when("testing versions", func() {
		when("there is no buildpack.yml", func() {
			it("shouldn't set the version in the buildplan", func() {
				code, err := runDetect(factory.Detect)
				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal(detect.PassStatusCode))

				Expect(factory.Output).To(Equal(buildplan.BuildPlan{
					php.Dependency: buildplan.Dependency{
						Version:  "",
						Metadata: buildplan.Metadata{},
					},
				}))
			})
		})

		when("there is a buildpack.yml", func() {
			const version string = "1.2.3"

			it.Before(func() {
				buildpackYAMLString := fmt.Sprintf("php:\n  version: %s", version)
				Expect(helper.WriteFile(filepath.Join(factory.Detect.Application.Root, "buildpack.yml"), 0666, buildpackYAMLString)).To(Succeed())
			})

			it("should pass with the requested version of php", func() {
				code, err := runDetect(factory.Detect)
				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal(detect.PassStatusCode))

				Expect(factory.Output).To(Equal(buildplan.BuildPlan{
					php.Dependency: buildplan.Dependency{
						Version:  version,
						Metadata: buildplan.Metadata{},
					},
				}))
			})
		})

		when("there is a is an existing version from the build plan and a buildpack.yml", func() {
			const buildpackYAMLVersion string = "1.2.3"
			const existingVersion string = "4.5.6"

			it.Before(func() {
				factory.AddBuildPlan(php.Dependency, buildplan.Dependency{
					Version: existingVersion,
				})

				buildpackYAMLString := fmt.Sprintf("php:\n  version: %s", buildpackYAMLVersion)
				Expect(helper.WriteFile(filepath.Join(factory.Detect.Application.Root, "buildpack.yml"), 0666, buildpackYAMLString)).To(Succeed())
			})

			it("should pass with the requested version of php defined in buildpack.yml", func() {
				code, err := runDetect(factory.Detect)
				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal(detect.PassStatusCode))

				Expect(factory.Output).To(Equal(buildplan.BuildPlan{
					php.Dependency: buildplan.Dependency{
						Version:  buildpackYAMLVersion,
						Metadata: buildplan.Metadata{},
					},
				}))
			})
		})
	})
}
