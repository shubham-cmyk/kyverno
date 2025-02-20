package processor

import (
	"fmt"
	"io"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/policy/annotations"
	"github.com/kyverno/kyverno/pkg/autogen"
	engineapi "github.com/kyverno/kyverno/pkg/engine/api"
	"k8s.io/api/admissionregistration/v1alpha1"
)

type ResultCounts struct {
	pass int
	fail int
	warn int
	err  int
	skip int
}

func (rc ResultCounts) Pass() int  { return rc.pass }
func (rc ResultCounts) Fail() int  { return rc.fail }
func (rc ResultCounts) Warn() int  { return rc.warn }
func (rc ResultCounts) Error() int { return rc.err }
func (rc ResultCounts) Skip() int  { return rc.skip }

func (rc *ResultCounts) addEngineResponses(auditWarn bool, responses ...engineapi.EngineResponse) {
	for _, response := range responses {
		rc.addEngineResponse(auditWarn, response)
	}
}

func (rc *ResultCounts) addEngineResponse(auditWarn bool, response engineapi.EngineResponse) {
	if !response.IsEmpty() {
		genericPolicy := response.Policy()
		if polType := genericPolicy.GetType(); polType == engineapi.ValidatingAdmissionPolicyType {
			return
		}
		policy := genericPolicy.GetPolicy().(kyvernov1.PolicyInterface)
		scored := annotations.Scored(policy.GetAnnotations())
		for _, rule := range autogen.ComputeRules(policy) {
			if rule.HasValidate() || rule.HasVerifyImageChecks() || rule.HasVerifyImages() {
				ruleFoundInEngineResponse := false
				for _, valResponseRule := range response.PolicyResponse.Rules {
					if rule.Name == valResponseRule.Name() {
						ruleFoundInEngineResponse = true
						switch valResponseRule.Status() {
						case engineapi.RuleStatusPass:
							rc.pass++
						case engineapi.RuleStatusFail:
							if !scored {
								rc.warn++
								break
							} else if auditWarn && response.GetValidationFailureAction().Audit() {
								rc.warn++
							} else {
								rc.fail++
							}
						case engineapi.RuleStatusError:
							rc.err++
						case engineapi.RuleStatusWarn:
							rc.warn++
						case engineapi.RuleStatusSkip:
							rc.skip++
						}
						continue
					}
				}
				if !ruleFoundInEngineResponse {
					rc.skip++
				}
			}
		}
	}
}

func (rc *ResultCounts) addGenerateResponse(out io.Writer, auditWarn bool, resPath string, response engineapi.EngineResponse) {
	genericPolicy := response.Policy()
	if polType := genericPolicy.GetType(); polType == engineapi.ValidatingAdmissionPolicyType {
		return
	}
	policy := genericPolicy.GetPolicy().(kyvernov1.PolicyInterface)
	printCount := 0
	for _, policyRule := range autogen.ComputeRules(policy) {
		ruleFoundInEngineResponse := false
		for i, ruleResponse := range response.PolicyResponse.Rules {
			if policyRule.Name == ruleResponse.Name() {
				ruleFoundInEngineResponse = true
				if ruleResponse.Status() == engineapi.RuleStatusPass {
					rc.pass++
				} else {
					if printCount < 1 {
						fmt.Fprintln(out, "\ninvalid resource", "policy", policy.GetName(), "resource", resPath)
						printCount++
					}
					fmt.Fprintf(out, "%d. %s - %s\n", i+1, ruleResponse.Name(), ruleResponse.Message())
					if auditWarn && response.GetValidationFailureAction().Audit() {
						rc.warn++
					} else {
						rc.fail++
					}
				}
				continue
			}
		}
		if !ruleFoundInEngineResponse {
			rc.skip++
		}
	}
}

func (rc *ResultCounts) addMutateResponse(out io.Writer, resourcePath string, response engineapi.EngineResponse) bool {
	genericPolicy := response.Policy()
	if polType := genericPolicy.GetType(); polType == engineapi.ValidatingAdmissionPolicyType {
		return false
	}
	policy := genericPolicy.GetPolicy().(kyvernov1.PolicyInterface)
	var policyHasMutate bool
	for _, rule := range autogen.ComputeRules(policy) {
		if rule.HasMutate() {
			policyHasMutate = true
		}
	}
	if !policyHasMutate {
		return false
	}
	printCount := 0
	printMutatedRes := false
	for _, policyRule := range autogen.ComputeRules(policy) {
		ruleFoundInEngineResponse := false
		for i, mutateResponseRule := range response.PolicyResponse.Rules {
			if policyRule.Name == mutateResponseRule.Name() {
				ruleFoundInEngineResponse = true
				if mutateResponseRule.Status() == engineapi.RuleStatusPass {
					rc.pass++
					printMutatedRes = true
				} else if mutateResponseRule.Status() == engineapi.RuleStatusSkip {
					fmt.Fprintf(out, "\nskipped mutate policy %s -> resource %s", policy.GetName(), resourcePath)
					rc.skip++
				} else if mutateResponseRule.Status() == engineapi.RuleStatusError {
					fmt.Fprintf(out, "\nerror while applying mutate policy %s -> resource %s\nerror: %s", policy.GetName(), resourcePath, mutateResponseRule.Message())
					rc.err++
				} else {
					if printCount < 1 {
						fmt.Fprintf(out, "\nfailed to apply mutate policy %s -> resource %s", policy.GetName(), resourcePath)
						printCount++
					}
					fmt.Fprintf(out, "%d. %s - %s \n", i+1, mutateResponseRule.Name(), mutateResponseRule.Message())
					rc.fail++
				}
				continue
			}
		}
		if !ruleFoundInEngineResponse {
			rc.skip++
		}
	}
	return printMutatedRes
}

func (rc *ResultCounts) addValidatingAdmissionResponse(vap v1alpha1.ValidatingAdmissionPolicy, engineResponse engineapi.EngineResponse) {
	for _, ruleResp := range engineResponse.PolicyResponse.Rules {
		if ruleResp.Status() == engineapi.RuleStatusPass {
			rc.pass++
		} else if ruleResp.Status() == engineapi.RuleStatusFail {
			rc.fail++
		} else if ruleResp.Status() == engineapi.RuleStatusError {
			rc.err++
		}
	}
}
