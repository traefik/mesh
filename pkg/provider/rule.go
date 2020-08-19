package provider

import (
	"fmt"
	"strings"

	"github.com/containous/maesh/pkg/topology"
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
)

func buildTrafficTargetRule(tt *topology.ServiceTrafficTarget) string {
	var orRules []string

	for _, rule := range tt.Rules {
		for _, match := range rule.HTTPMatches {
			var matchParts []string

			// Handle Path filtering.
			matchParts = appendPathFilter(matchParts, match)

			// Handle Method filtering.
			matchParts = appendMethodFilter(matchParts, match)

			// Handle Header filtering.
			matchParts = appendHeaderFilter(matchParts, match)

			// Conditions within a HTTPMatch must all be fulfilled to be considered valid.
			if len(matchParts) > 0 {
				matchCond := strings.Join(matchParts, " && ")
				if len(matchParts) > 1 {
					matchCond = fmt.Sprintf("(%s)", matchCond)
				}

				orRules = append(orRules, matchCond)
			}
		}
	}

	// At least one HTTPMatch in the Specs must be valid.
	return strings.Join(orRules, " || ")
}

func appendPathFilter(matchParts []string, match *specs.HTTPMatch) []string {
	if match.PathRegex == "" {
		return matchParts
	}

	pathRegex := match.PathRegex
	if strings.HasPrefix(match.PathRegex, "/") {
		pathRegex = strings.TrimPrefix(match.PathRegex, "/")
	}

	return append(matchParts, fmt.Sprintf("PathPrefix(`/{path:%s}`)", pathRegex))
}

func appendMethodFilter(matchParts []string, match *specs.HTTPMatch) []string {
	if len(match.Methods) == 0 {
		return matchParts
	}

	var matchAll bool

	for _, m := range match.Methods {
		if m == "*" {
			matchAll = true
			break
		}
	}

	if !matchAll {
		methods := strings.Join(match.Methods, "`,`")
		return append(matchParts, fmt.Sprintf("Method(`%s`)", methods))
	}

	return matchParts
}

func appendHeaderFilter(matchParts []string, match *specs.HTTPMatch) []string {
	rules := make([]string, 0, len(match.Headers))

	for name, value := range match.Headers {
		rules = append(rules, fmt.Sprintf("HeadersRegexp(`%s`, `%s`)", name, value))
	}

	if len(rules) > 0 {
		matchParts = append(matchParts, strings.Join(rules, " && "))
	}

	return matchParts
}

func buildHTTPRuleFromService(svc *topology.Service) string {
	return fmt.Sprintf("Host(`%s.%s.maesh`) || Host(`%s`)", svc.Name, svc.Namespace, svc.ClusterIP)
}

func buildHTTPRuleFromTrafficTarget(tt *topology.ServiceTrafficTarget, ttSvc *topology.Service) string {
	ttRule := buildTrafficTargetRule(tt)
	httpRule := buildHTTPRuleFromService(ttSvc)

	if ttRule != "" {
		return fmt.Sprintf("(%s) && (%s)", httpRule, ttRule)
	}

	return httpRule
}

func buildHTTPRuleFromTrafficTargetIndirect(tt *topology.ServiceTrafficTarget, ttSvc *topology.Service) string {
	ttRule := buildTrafficTargetRule(tt)
	svcRule := buildHTTPRuleFromService(ttSvc)
	indirectRule := "HeadersRegexp(`X-Forwarded-For`, `.+`)"

	if ttRule != "" {
		return fmt.Sprintf("(%s) && (%s) && %s", svcRule, ttRule, indirectRule)
	}

	return fmt.Sprintf("(%s) && %s", svcRule, indirectRule)
}

func buildHTTPRuleFromTrafficSplitIndirect(tsSvc *topology.Service) string {
	svcRule := buildHTTPRuleFromService(tsSvc)
	indirectRule := "HeadersRegexp(`X-Forwarded-For`, `.+`)"

	return fmt.Sprintf("(%s) && %s", svcRule, indirectRule)
}

func buildTCPRouterRule() string {
	return "HostSNI(`*`)"
}

func getRulePriority(rule string, priority int) int {
	andOps := strings.Count(rule, "&&")
	orOps := strings.Count(rule, "||")

	return priority*1000 + (andOps + orOps)
}
