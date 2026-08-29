package organizer

import (
	"sort"
	"strings"
	"unicode"
)

type ClusterInfo struct {
	Canonical string   `json:"canonical"`
	Aliases   []string `json:"aliases,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Score     float64  `json:"score,omitempty"`
}

func clusterPlans(plans []Plan, persisted map[string]string) []Plan {
	if len(plans)<2&&len(persisted)==0{return plans}
	for i:=range plans{if plans[i].Series==""||plans[i].Error!=""{continue};if canonical,ok:=aliasLookup(persisted,plans[i].Series);ok{plans[i].Cluster=ClusterInfo{Canonical:canonical,Aliases:[]string{plans[i].Series},Reason:"saved alias",Score:1};plans[i].Series=canonical}}
	parent:=make([]int,len(plans));for i:=range parent{parent[i]=i};var find func(int)int;find=func(x int)int{if parent[x]!=x{parent[x]=find(parent[x])};return parent[x]};union:=func(a,b int){ra,rb:=find(a),find(b);if ra!=rb{parent[rb]=ra}}
	for i:=0;i<len(plans);i++{if plans[i].Series==""||plans[i].Error!=""{continue};for j:=i+1;j<len(plans);j++{if plans[j].Series==""||plans[j].Error!=""{continue};score,reason:=sameSeries(plans[i].Series,plans[j].Series);if score>=0.90{union(i,j);if plans[i].Cluster.Reason==""{plans[i].Cluster.Reason=reason;plans[i].Cluster.Score=score};if plans[j].Cluster.Reason==""{plans[j].Cluster.Reason=reason;plans[j].Cluster.Score=score}}}}
	groups:=map[int][]int{};for i:=range plans{if plans[i].Series!=""&&plans[i].Error==""{r:=find(i);groups[r]=append(groups[r],i)}}
	for _,idxs:=range groups{if len(idxs)<2{continue};canonical:=chooseCanonical(plans,idxs,persisted);aliases:=make([]string,0,len(idxs));seen:=map[string]struct{}{};for _,idx:=range idxs{if _,ok:=seen[plans[idx].Series];!ok{seen[plans[idx].Series]=struct{}{};aliases=append(aliases,plans[idx].Series)}};sort.Slice(aliases,func(i,j int)bool{return strings.ToLower(aliases[i])<strings.ToLower(aliases[j])});for _,idx:=range idxs{old:=plans[idx].Series;plans[idx].Series=canonical;plans[idx].Destination=replaceSeriesDir(plans[idx].Destination,old,canonical);plans[idx].Cluster.Canonical=canonical;plans[idx].Cluster.Aliases=aliases;if plans[idx].Cluster.Reason==""{plans[idx].Cluster.Reason="series cluster";plans[idx].Cluster.Score=0.95}}}
	return plans
}

func aliasLookup(aliases map[string]string,s string)(string,bool){key:=canonicalKey(s);for a,c:=range aliases{if canonicalKey(a)==key{return c,true}};return "",false}
func chooseCanonical(plans []Plan,idxs []int,persisted map[string]string)string{for _,idx:=range idxs{if c,ok:=aliasLookup(persisted,plans[idx].Series);ok{return c}};best:=plans[idxs[0]].Series;for _,idx:=range idxs[1:]{cand:=plans[idx].Series;if richerSeries(cand,best){best=cand}};return best}
func richerSeries(a,b string)bool{aj,bj:=containsJapaneseText(a),containsJapaneseText(b);al,bl:=containsLatin(a),containsLatin(b);if aj&&al&&!(bj&&bl){return true};if bj&&bl&&!(aj&&al){return false};return len([]rune(a))>len([]rune(b))}

func sameSeries(a,b string)(float64,string){ka,kb:=canonicalKey(a),canonicalKey(b);if ka==""||kb==""{return 0,""};if ka==kb{return 1,"normalized exact match"};if bilingualEquivalent(a,b){return 0.98,"latin title plus Japanese alias"};if hasSpinOffMarker(a)||hasSpinOffMarker(b){return 0,""};la,lb:=len([]rune(ka)),len([]rune(kb));s:=levenshteinSimilarity([]rune(ka),[]rune(kb));if la>=10&&lb>=10&&s>=0.90{return s,"minor filename variation"};if la>=8&&lb>=8&&s>=0.94{return s,"minor filename variation"};return 0,""}
func bilingualEquivalent(a,b string)bool{if hasSpinOffMarker(a)||hasSpinOffMarker(b){return false};short,long:=a,b;if len([]rune(short))>len([]rune(long)){short,long=long,short};sk:=canonicalKey(short);if len([]rune(sk))<5||!containsLatin(short){return false};lk:=canonicalKey(long);if !strings.HasPrefix(lk,sk){return false};rest:=strings.TrimPrefix(lk,sk);return rest!=""&&containsJapaneseText(rest)}
func canonicalKey(s string)string{s=strings.ToLower(strings.TrimSpace(s));s=strings.Map(func(r rune)rune{switch r{case ' ','\t','_','-','‐','‑','–','—','・','.','．','·','!','！','?','？',',','，','、','(',')','（','）','[',']','【','】':return -1};if r>='Ａ'&&r<='Ｚ'{return r-'Ａ'+'a'};if r>='ａ'&&r<='ｚ'{return r-'ａ'+'a'};if r>='０'&&r<='９'{return r-'０'+'0'};return r},s);return s}
func hasSpinOffMarker(s string)bool{l:=strings.ToLower(s);for _,m:=range []string{"外伝","番外","スピンオフ","spin-off","spinoff","side story","another story"}{if strings.Contains(l,m){return true}};return false}
func containsJapaneseText(s string)bool{for _,r:=range s{if unicode.In(r,unicode.Hiragana,unicode.Katakana)||(r>=0x3400&&r<=0x9fff){return true}};return false}
func containsLatin(s string)bool{for _,r:=range s{if(r>='a'&&r<='z')||(r>='A'&&r<='Z')||(r>='Ａ'&&r<='Ｚ')||(r>='ａ'&&r<='ｚ'){return true}};return false}
func levenshteinSimilarity(a,b []rune)float64{m,n:=len(a),len(b);if m==0||n==0{return 0};prev:=make([]int,n+1);cur:=make([]int,n+1);for j:=0;j<=n;j++{prev[j]=j};for i:=1;i<=m;i++{cur[0]=i;for j:=1;j<=n;j++{cost:=0;if a[i-1]!=b[j-1]{cost=1};x,y,z:=prev[j]+1,cur[j-1]+1,prev[j-1]+cost;if y<x{x=y};if z<x{x=z};cur[j]=x};prev,cur=cur,prev};max:=m;if n>max{max=n};return 1-float64(prev[n])/float64(max)}
func replaceSeriesDir(destination,oldSeries,newSeries string)string{if destination==""||oldSeries==newSeries{return destination};dir:=strings.TrimSuffix(destination,"/"+pathBase(destination));if strings.HasSuffix(dir,"/"+oldSeries){return strings.TrimSuffix(dir,oldSeries)+newSeries+"/"+pathBase(destination)};return destination}
func pathBase(s string)string{if i:=strings.LastIndex(s,"/");i>=0{return s[i+1:]};return s}
