package organizer

import (
	"path/filepath"
	"testing"
)

func TestApplyDestinationKeepsBilingualAliasVolumesInCanonicalSeriesFolder(t *testing.T) {
	root := t.TempDir()
	o, err := New(Config{Root: root, OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "BLACK_LAGOON_01b-03b.rar", Series: "BLACK LAGOON ブラック・ラグーン", Cluster: ClusterInfo{Aliases: []string{"BLACK LAGOON", "BLACK LAGOON ブラック・ラグーン"}}, PreviewGroups: []string{"[広江礼威] BLACK LAGOON 第01巻", "[広江礼威] BLACK LAGOON 第02巻", "[広江礼威] BLACK LAGOON 第03巻"}, PreviewNested: true}
	if err := o.applyDestination(&plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.PredictedOutputs) != 3 {
		t.Fatalf("outputs=%#v", plan.PredictedOutputs)
	}
	for _, target := range plan.PredictedOutputs {
		if got := filepath.Base(filepath.Dir(target)); got != plan.Series {
			t.Fatalf("target escaped canonical series folder: %s", target)
		}
	}
}

func TestMarkArchiveOutputConflictsAnnotatesBothPlans(t *testing.T) {
	target := "/share/Download/Temp/作品名/作品名 第01巻.zip"
	plans := []Plan{{Name: "all.rar", PredictedOutputs: []string{target}}, {Name: "patch.rar", PredictedOutputs: []string{target}}}
	markArchiveOutputConflicts(plans)
	for _, plan := range plans {
		if plan.PreviewWarning == "" {
			t.Fatalf("missing warning: %#v", plan)
		}
	}
}
