package organizer

import "testing"

func TestSameSeriesNormalizesPunctuation(t *testing.T){score,_:=sameSeries("BLACK_LAGOON","Black Lagoon");if score<0.99{t.Fatalf("score=%v",score)}}
func TestSameSeriesBilingualAlias(t *testing.T){score,reason:=sameSeries("BLACK LAGOON","BLACK LAGOON ブラック・ラグーン");if score<0.97{t.Fatalf("score=%v reason=%s",score,reason)}}
func TestSameSeriesMinorVariation(t *testing.T){score,_:=sameSeries("灰宮先輩は怖くてかわいい","灰宮先輩は怖くてかわいぃ");if score<0.94{t.Fatalf("score=%v",score)}}
func TestSpinOffDoesNotAutoMerge(t *testing.T){score,_:=sameSeries("BLACK LAGOON","BLACK LAGOON 外伝");if score!=0{t.Fatalf("spin-off should not merge: %v",score)}}
func TestClusterChoosesBilingualCanonical(t *testing.T){plans:=[]Plan{{Series:"BLACK LAGOON",Destination:"/share/x/BLACK LAGOON/a.zip"},{Series:"BLACK LAGOON ブラック・ラグーン",Destination:"/share/x/BLACK LAGOON ブラック・ラグーン/b.zip"}};got:=clusterPlans(plans,nil);for _,p:=range got{if p.Series!="BLACK LAGOON ブラック・ラグーン"{t.Fatalf("unexpected canonical: %q",p.Series)}}}
func TestPersistedAliasWins(t *testing.T){plans:=[]Plan{{Series:"Black_Lagoon",Destination:"/share/x/Black_Lagoon/a.zip"}};got:=clusterPlans(plans,map[string]string{"Black Lagoon":"BLACK LAGOON ブラック・ラグーン"});if got[0].Series!="BLACK LAGOON ブラック・ラグーン"{t.Fatalf("got %q",got[0].Series)}}
