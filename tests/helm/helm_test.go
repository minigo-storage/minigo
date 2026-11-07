package helm_test

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func object(v any) map[string]any { return v.(map[string]any) }

func TestHardenedChartWorkloads(t *testing.T) {
	for _, tc := range []struct{ mode, persistence, drives, tls string }{
		{"standalone", "true", "1", "false"},
		{"standalone", "false", "1", "true"},
		{"distributed", "true", "2", "true"},
		{"distributed", "false", "2", "false"},
	} {
		t.Run(tc.mode+"/persistent="+tc.persistence+"/tls="+tc.tls, func(t *testing.T) {
			cmd := exec.Command("helm", "template", "security-test", "../../helm/minio",
				"--set", "mode="+tc.mode, "--set", "persistence.enabled="+tc.persistence,
				"--set", "drivesPerNode="+tc.drives, "--set", "tls.enabled="+tc.tls,
				"--set", "buckets[0].name=security-fixture")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("helm: %v: %s", err, output)
			}
			if bytes.Contains(output, []byte("console123")) || bytes.Contains(output, []byte("mc --insecure")) {
				t.Fatal("chart enables default credentials or disables TLS verification")
			}
			decoder := yaml.NewDecoder(bytes.NewReader(output))
			workloads := 0
			for {
				var doc map[string]any
				if err := decoder.Decode(&doc); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				kind := doc["kind"]
				if kind != "StatefulSet" && kind != "Deployment" && kind != "Job" {
					continue
				}
				workloads++
				spec := object(doc["spec"])
				pod := object(object(spec["template"])["spec"])
				security := object(pod["securityContext"])
				if security["runAsNonRoot"] != true || security["runAsUser"] != 1000 || object(security["seccompProfile"])["type"] != "RuntimeDefault" {
					t.Fatalf("%s lacks restricted pod security: %v", kind, security)
				}
				volumes := map[string]bool{}
				for _, v := range pod["volumes"].([]any) {
					volumes[object(v)["name"].(string)] = true
				}
				if claims, ok := spec["volumeClaimTemplates"].([]any); ok {
					for _, v := range claims {
						volumes[object(object(v)["metadata"])["name"].(string)] = true
					}
				}
				containers := append([]any{}, pod["containers"].([]any)...)
				if init, ok := pod["initContainers"].([]any); ok {
					containers = append(containers, init...)
				}
				for _, v := range containers {
					container := object(v)
					if !strings.HasPrefix(container["image"].(string), "minigostorage/minigo:") {
						t.Fatal("workload uses an unpatched upstream image")
					}
					security := object(container["securityContext"])
					if security["readOnlyRootFilesystem"] != true || security["allowPrivilegeEscalation"] != false || object(security["capabilities"])["drop"].([]any)[0] != "ALL" {
						t.Fatalf("%s lacks restricted container security: %v", kind, security)
					}
					for _, mount := range container["volumeMounts"].([]any) {
						name := object(mount)["name"].(string)
						if !volumes[name] {
							t.Fatalf("%s mounts undefined volume %q", kind, name)
						}
					}
				}
			}
			if workloads != 2 {
				t.Fatalf("expected storage controller and setup job; got %d workloads", workloads)
			}
		})
	}
}
