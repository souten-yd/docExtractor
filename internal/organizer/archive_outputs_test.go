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

func TestApplyDestinationKeepsSoleTransliteratedGroupInCanonicalSeriesFolder(t *testing.T) {
	root := t.TempDir()
	o, err := New(Config{Root: root, OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "Tensei_shitara_dai_nana_oujidattanode_23.rar", Series: "転生したら第七王子だったので、気ままに魔術を極めます", Confidence: .99, PreviewGroups: []string{"Tensei_shitara_dai_nana_oujidattanode_23"}}
	if err := o.applyDestination(&plan); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(filepath.Dir(plan.PredictedOutputs[0])); got != plan.Series {
		t.Fatalf("target folder=%q want=%q", got, plan.Series)
	}
}

func TestApplyDestinationKeepsSoleChapterRangeGroupInCanonicalSeriesFolder(t *testing.T) {
	root := t.TempDir()
	o, err := New(Config{Root: root, OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "Yuushasama_08.rar", Series: "勇者さまは報酬に人妻をご希望です", Confidence: .99, PreviewGroups: []string{"勇者さまは報酬に人妻をご希望です 第39-41話"}}
	if err := o.applyDestination(&plan); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(filepath.Dir(plan.PredictedOutputs[0])); got != plan.Series {
		t.Fatalf("target folder=%q want=%q", got, plan.Series)
	}
}

func TestApplyDestinationDoesNotForceMixedWorksIntoOneSeries(t *testing.T) {
	root := t.TempDir()
	o, err := New(Config{Root: root, OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "mixed.rar", Series: "作品A", Confidence: .99, PreviewGroups: []string{"作品A 第01巻", "作品B 第01巻"}}
	if err := o.applyDestination(&plan); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(filepath.Dir(plan.PredictedOutputs[1])); got != "作品B" {
		t.Fatalf("mixed work was forced into canonical folder: %s", plan.PredictedOutputs[1])
	}
}

func TestApplyDestinationKeepsMultipleTransliteratedVolumesTogether(t *testing.T) {
	root := t.TempDir()
	o, err := New(Config{Root: root, OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Name: "collection.rar", Series: "日本語の正規作品名", Confidence: .99, PreviewGroups: []string{"Romanized_Work_01", "Romanized_Work_02", "Romanized_Work_03"}}
	if err := o.applyDestination(&plan); err != nil {
		t.Fatal(err)
	}
	for _, target := range plan.PredictedOutputs {
		if got := filepath.Base(filepath.Dir(target)); got != plan.Series {
			t.Fatalf("target folder=%q want=%q: %s", got, plan.Series, target)
		}
	}
}

func TestArchiveGroupsDescribeOneWorkAcceptsChapterRangesButRejectsMixedTitles(t *testing.T) {
	for _, groups := range [][]string{
		{"長い作品タイトル 第01-03話", "長い作品タイトル 第04-06話"},
		{"長い作品タイトル 第01話-第03話", "長い作品タイトル 第04話-第06話"},
		{"Long Work ch.001-003", "Long Work ch.004-006"},
		{"長い作品タイトル 01-03巻", "長い作品タイトル 04-06巻"},
	} {
		if !archiveGroupsDescribeOneWork(groups) {
			t.Fatalf("coverage ranges of one work were not grouped: %#v", groups)
		}
	}
	if archiveGroupsDescribeOneWork([]string{"まったく異なる作品A 第01巻", "別の作品B 第01巻"}) {
		t.Fatal("mixed works were grouped")
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
