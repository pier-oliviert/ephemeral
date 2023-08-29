package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Build Types", func() {

	Context("BuildConditions", func() {
		It("Returns BuildPhaseDone when all condition are successful", func() {
			Expect(BuildStatus{
				Conditions: BuildConditions{
					BuildCondition{
						Type:   "warmup",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionSuccess,
					},
				},
			}.Conditions.Phase()).To(Equal(BuildPhaseDone))
		})

		It("Returns BuildPhaseRunning when at least one condition is in progress", func() {
			Expect(BuildStatus{
				Conditions: BuildConditions{
					BuildCondition{
						Type:   "warmup",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionInProgress,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionWaiting,
					},
				},
			}.Conditions.Phase()).To(Equal(BuildPhaseRunning))

			Expect(BuildStatus{
				Conditions: BuildConditions{
					BuildCondition{
						Type:   "warmup",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionInProgress,
					},
				},
			}.Conditions.Phase()).To(Equal(BuildPhaseRunning))
		})

		It("Returns BuildPhaseError when at least one condition failed", func() {
			Expect(BuildStatus{
				Conditions: BuildConditions{
					BuildCondition{
						Type:   "warmup",
						Status: ConditionSuccess,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionError,
					},
					BuildCondition{
						Type:   "build",
						Status: ConditionWaiting,
					},
				},
			}.Conditions.Phase()).To(Equal(BuildPhaseError))
		})
	})
})
