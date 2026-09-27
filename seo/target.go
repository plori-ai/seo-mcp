package seo

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

type researchTarget struct {
	scope, hostname, urlHostname, path, display string
}

var targetProtocol = regexp.MustCompile(`^[a-zA-Z][a-zA-Z\d+.-]*://`)
var targetHostChars = regexp.MustCompile(`^[a-z\d.-]+$`)
var rankedBareDomain = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)
var rankedAbsoluteURL = regexp.MustCompile(`^https?://\S+$`)

func parseResearchTarget(input, scope string) (researchTarget, error) {
	var target researchTarget
	if scope != "" && scope != "domain" && scope != "subdomains" && scope != "subfolder" && scope != "exact_url" {
		return target, inputErrorf("Invalid scope: %s", scope)
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return target, inputErrorf("Enter a domain or URL")
	}
	if !targetProtocol.MatchString(trimmed) {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return target, inputErrorf("Enter a valid domain like example.com")
	}
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		if parsed.User.Username() != "" || password != "" {
			return target, inputErrorf("URLs with embedded credentials are not supported")
		}
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value > 65535 {
			return target, inputErrorf("Enter a valid domain like example.com")
		}
	}
	hostname, err := asciiHostname(parsed.Hostname())
	if err != nil {
		return target, inputErrorf("Enter a valid domain like example.com")
	}
	target.urlHostname = strings.ToLower(hostname)
	target.hostname = strings.TrimPrefix(target.urlHostname, "www.")
	host := strings.TrimRight(target.hostname, ".")
	labels := strings.Split(host, ".")
	if !targetHostChars.MatchString(target.hostname) || len(labels) < 2 || len(host) > 255 || strings.HasPrefix(host, "-") || net.ParseIP(host) != nil || !validResearchSuffix(host) {
		return researchTarget{}, inputErrorf("Enter a valid domain like example.com")
	}
	for i, label := range labels {
		if (label == "" && i != 0) || len(label) > 63 || strings.HasSuffix(label, "-") {
			return researchTarget{}, inputErrorf("Enter a valid domain like example.com")
		}
	}
	target.path = normalizeResearchPath(parsed.EscapedPath())
	if scope == "subfolder" && target.path == "" {
		return researchTarget{}, inputErrorf("Add a path to use Subfolder (e.g. example.com/blog)")
	}
	if scope == "" {
		scope = "subdomains"
		if target.path != "" {
			scope = "subfolder"
		}
	}
	target.scope = scope
	target.display = target.hostname
	if scope == "subfolder" || scope == "exact_url" {
		target.display += target.path
	}
	return target, nil
}

// URL's dot-segment normalization keeps duplicate slashes and encoded path
// bytes intact; path.Clean would change both research targets and filters.
func normalizeResearchPath(path string) string {
	var parts []string
	for _, part := range strings.Split(path, "/") {
		dot := strings.ReplaceAll(strings.ToLower(part), "%2e", ".")
		switch dot {
		case ".":
			continue
		case "..":
			if len(parts) > 1 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, part)
		}
	}
	return strings.TrimRight(strings.Join(parts, "/"), "/")
}

type backlinksTarget struct {
	apiTarget, display, scope, path string
	includeSubdomains               bool
}

func normalizeBacklinksTarget(input, scope string) (backlinksTarget, error) {
	if scope == "page" {
		scope = "exact_url"
	}
	target, err := parseResearchTarget(input, scope)
	if err != nil {
		return backlinksTarget{}, err
	}
	result := backlinksTarget{apiTarget: target.hostname, display: target.display, scope: target.scope, includeSubdomains: target.scope == "subdomains"}
	if target.scope == "subfolder" {
		result.path = target.path
	}
	if target.scope == "exact_url" {
		if strings.ContainsAny(strings.TrimSpace(input), "?#") {
			return backlinksTarget{}, inputErrorf("Page URLs with query strings or fragments are not supported")
		}
		protocol := "https://"
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "http://") {
			protocol = "http://"
		}
		path := target.path
		if path == "" {
			path = "/"
		}
		result.apiTarget = protocol + target.urlHostname + path
		result.display = result.apiTarget
		result.includeSubdomains = true
	}
	return result, nil
}

func escapeResearchLike(term string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
}

func joinResearchClauses(clauses []any, operator string) []any {
	joined := make([]any, 0, len(clauses)*2)
	for _, clause := range clauses {
		if len(joined) > 0 {
			joined = append(joined, operator)
		}
		joined = append(joined, clause)
	}
	return joined
}

func rankedScopeClauses(target researchTarget) ([]any, int) {
	if target.scope == "subdomains" {
		return nil, 0
	}
	clauses := []any{[]any{"ranked_serp_element.serp_item.domain", "in", []string{target.hostname, "www." + target.hostname}}}
	if target.scope == "domain" {
		return clauses, 1
	}
	path := target.path
	if path == "" {
		path = "/"
	}
	escaped := escapeResearchLike(path)
	field := "ranked_serp_element.serp_item.relative_url"
	var paths []any
	if target.scope == "subfolder" {
		paths = []any{[]any{field, "=", path}, []any{field, "like", escaped + "/%"}, []any{field, "like", escaped + "?%"}}
	} else {
		paths = []any{[]any{field, "in", []string{path, path + "/"}}, []any{field, "like", escaped + "?%"}, []any{field, "like", escaped + "/?%"}}
	}
	return append(clauses, joinResearchClauses(paths, "or")), 4
}

func backlinksScopeClauses(target backlinksTarget) []any {
	var clauses []any
	for _, host := range []string{target.apiTarget, "www." + target.apiTarget} {
		prefix := "%://" + escapeResearchLike(host) + escapeResearchLike(target.path)
		clauses = append(clauses, []any{"url_to", "like", prefix}, []any{"url_to", "like", prefix + "/%"})
	}
	return []any{joinResearchClauses(clauses, "or")}
}

// Recognized suffix roots from tldts 7.0.25, the version used by the
// reference implementation (https://github.com/remusao/tldts, MIT). Only
// suffix recognition is needed here, not registrable-domain extraction.
//
// # Copyright (c) 2017 Thomas Parisot, 2018 Rémi Berson
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.
const researchTLDs = `
aaa aarp abb abbott abbvie abc able abogado abudhabi ac academy accenture accountant accountants aco
actor ad ads adult ae aeg aero aetna af afl africa ag agakhan agency ai aig airbus airforce airtel
akdn al alibaba alipay allfinanz allstate ally alsace alstom am amazon americanexpress
americanfamily amex amfam amica amsterdam analytics android anquan anz ao aol apartments app apple
aq aquarelle ar arab aramco archi army arpa art arte as asda asia associates at athleta attorney au
auction audi audible audio auspost author auto autos aw aws ax axa az azure ba baby baidu banamex
band bank bar barcelona barclaycard barclays barefoot bargains baseball basketball bauhaus bayern bb
bbc bbt bbva bcg bcn bd be beats beauty beer berlin best bestbuy bet bf bg bh bharti bi bible bid
bike bing bingo bio biz bj black blackfriday blockbuster blog bloomberg blue bm bms bmw bn
bnpparibas bo boats boehringer bofa bom bond boo book booking bosch bostik boston bot boutique box
br bradesco bridgestone broadway broker brother brussels bs bt build builders business buy buzz bv
bw by bz bzh ca cab cafe cal call calvinklein cam camera camp canon capetown capital capitalone car
caravan cards care career careers cars casa case cash casino cat catering catholic cba cbn cbre cc
cd center ceo cern cf cfa cfd cg ch chanel channel charity chase chat cheap chintai christmas chrome
church ci cipriani circle cisco citadel citi citic city cl claims cleaning click clinic clinique
clothing cloud club clubmed cm cn co coach codes coffee college cologne com commbank community
company compare computer comsec condos construction consulting contact contractors cooking cool coop
corsica country coupon coupons courses cpa cr credit creditcard creditunion cricket crown crs cruise
cruises cu cuisinella cv cw cx cy cymru cyou cz dad dance data date dating datsun day dclk dds de
deal dealer deals degree delivery dell deloitte delta democrat dental dentist desi design dev dhl
diamonds diet digital direct directory discount discover dish diy dj dk dm dnp do docs doctor dog
domains dot download drive dtv dubai dupont durban dvag dvr dz earth eat ec eco edeka edu education
ee eg email emerck energy engineer engineering enterprises epson equipment ericsson erni es esq
estate et eu eurovision eus events exchange expert exposed express extraspace fage fail fairwinds
faith family fan fans farm farmers fashion fast fedex feedback ferrari ferrero fi fidelity fido film
final finance financial fire firestone firmdale fish fishing fit fitness fj flickr flights flir
florist flowers fly fm fo foo food football ford forex forsale forum foundation fox fr free
fresenius frl frogans frontier ftr fujitsu fun fund furniture futbol fyi ga gal gallery gallo gallup
game games gap garden gay gb gbiz gd gdn ge gea gent genting george gf gg ggee gh gi gift gifts
gives giving gl glass gle global globo gm gmail gmbh gmo gmx gn godaddy gold goldpoint golf goodyear
goog google gop got gov gp gq gr grainger graphics gratis green gripe grocery group gs gt gu gucci
guge guide guitars guru gw gy hair hamburg hangout haus hbo hdfc hdfcbank health healthcare help
helsinki here hermes hiphop hisamitsu hitachi hiv hk hkt hm hn hockey holdings holiday homedepot
homegoods homes homesense honda horse hospital host hosting hot hotel hotels hotmail house how hr
hsbc ht hu hughes hyatt hyundai ibm icbc ice icu id ie ieee ifm ikano il im imamat imdb immo
immobilien in inc industries infiniti info ing ink institute insurance insure int international
intuit investments io ipiranga iq ir irish is ismaili ist istanbul it itau itv jaguar java jcb je
jeep jetzt jewelry jio jll jmp jnj jo jobs joburg jot joy jp jpmorgan jprs juegos juniper kaufen
kddi ke kerryhotels kerryproperties kfh kg kh ki kia kids kim kindle kitchen kiwi km kn koeln
komatsu kosher kp kpmg kpn kr krd kred kuokgroup kw ky kyoto kz la lacaixa lamborghini lamer land
landrover lanxess lasalle lat latino latrobe law lawyer lb lc lds lease leclerc lefrak legal lego
lexus lgbt li lidl life lifeinsurance lifestyle lighting like lilly limited limo lincoln link live
living lk llc llp loan loans locker locus lol london lotte lotto love lpl lplfinancial lr ls lt ltd
ltda lu lundbeck luxe luxury lv ly ma madrid maif maison makeup man management mango map market
marketing markets marriott marshalls mattel mba mc mckinsey md me med media meet melbourne meme
memorial men menu merck merckmsd mg mh miami microsoft mil mini mint mit mitsubishi mk ml mlb mls
mma mn mo mobi mobile moda moe moi mom monash money monster mormon mortgage moscow moto motorcycles
mov movie mp mq mr ms msd mt mtn mtr mu museum music mv mw mx my mz na nab nagoya name navy nba nc
ne nec net netbank netflix network neustar new news next nextdirect nexus nf nfl ng ngo nhk ni nico
nike nikon ninja nissan nissay nl no nokia norton now nowruz nowtv nr nra nrw ntt nu nyc nz obi
observer office okinawa olayan olayangroup ollo om omega one ong onion onl online ooo open oracle
orange org organic origins osaka otsuka ott ovh pa page panasonic paris pars partners parts party
pay pccw pe pet pf pfizer ph pharmacy phd philips phone photo photography photos physio pics pictet
pictures pid pin ping pink pioneer pizza pk pl place play playstation plumbing plus pm pn pnc pohl
poker politie porn post pr praxi press prime pro prod productions prof progressive promo properties
property protection pru prudential ps pt pub pw pwc py qa qpon quebec quest racing radio re read
realestate realtor realty recipes red redumbrella rehab reise reisen reit reliance ren rent rentals
repair report republican rest restaurant review reviews rexroth rich richardli ricoh ril rio rip ro
rocks rodeo rogers room rs rsvp ru rugby ruhr run rw rwe ryukyu sa saarland safe safety sakura sale
salon samsclub samsung sandvik sandvikcoromant sanofi sap sarl sas save saxo sb sbi sbs sc scb
schaeffler schmidt scholarships school schule schwarz science scot sd se search seat secure security
seek select sener services seven sew sex sexy sfr sg sh shangrila sharp shell shia shiksha shoes
shop shopping shouji show si silk sina singles site sj sk ski skin sky skype sl sling sm smart smile
sn sncf so soccer social softbank software sohu solar solutions song sony soy spa space sport spot
sr srl ss st stada staples star statebank statefarm stc stcgroup stockholm storage store stream
studio study style su sucks supplies supply support surf surgery suzuki sv swatch swiss sx sy sydney
systems sz tab taipei talk taobao target tatamotors tatar tattoo tax taxi tc tci td tdk team tech
technology tel temasek tennis teva tf tg th thd theater theatre tiaa tickets tienda tips tires tirol
tj tjmaxx tjx tk tkmaxx tl tm tmall tn to today tokyo tools top toray toshiba total tours town
toyota toys tr trade trading training travel travelers travelersinsurance trust trv tt tube tui
tunes tushu tv tvs tw tz ua ubank ubs ug uk unicom university uno uol ups us uy uz va vacations vana
vanguard vc ve vegas ventures verisign versicherung vet vg vi viajes video vig viking villas vin vip
virgin visa vision viva vivo vlaanderen vn vodka volvo vote voting voto voyage vu wales walmart
walter wang wanggou watch watches weather weatherchannel webcam weber website wed wedding weibo weir
wf whoswho wien wiki williamhill win windows wine winners wme woodside work works world wow ws wtc
wtf xbox xerox xihuan xin xn--11b4c3d xn--11b4c3d xn--1ck2e1b xn--1ck2e1b xn--1qqw23a xn--1qqw23a xn
--2scrj9c xn--2scrj9c xn--30rr7y xn--30rr7y xn--3bst00m xn--3bst00m xn--3ds443g xn--3ds443g xn--
3e0b707e xn--3e0b707e xn--3hcrj9c xn--3hcrj9c xn--3pxu8k xn--3pxu8k xn--42c2d9a xn--42c2d9a xn--
45br5cyl xn--45br5cyl xn--45brj9c xn--45brj9c xn--45q11c xn--45q11c xn--4dbrk0ce xn--4dbrk0ce xn--
4gbrim xn--4gbrim xn--54b7fta0cc xn--54b7fta0cc xn--55qw42g xn--55qw42g xn--55qx5d xn--55qx5d xn--
5su34j936bgsg xn--5su34j936bgsg xn--5tzm5g xn--5tzm5g xn--6frz82g xn--6frz82g xn--6qq986b3xl xn--
6qq986b3xl xn--80adxhks xn--80adxhks xn--80ao21a xn--80ao21a xn--80aqecdr1a xn--80aqecdr1a xn--
80asehdb xn--80asehdb xn--80aswg xn--80aswg xn--8y0a063a xn--8y0a063a xn--90a3ac xn--90a3ac xn--90ae
xn--90ae xn--90ais xn--90ais xn--9dbq2a xn--9dbq2a xn--9et52u xn--9et52u xn--9krt00a xn--9krt00a xn
--b4w605ferd xn--b4w605ferd xn--bck1b9a5dre4c xn--bck1b9a5dre4c xn--c1avg xn--c1avg xn--c2br7g xn--
c2br7g xn--cck2b3b xn--cck2b3b xn--cckwcxetd xn--cckwcxetd xn--cg4bki xn--cg4bki xn--
clchc0ea0b2g2a9gcd xn--clchc0ea0b2g2a9gcd xn--czr694b xn--czr694b xn--czrs0t xn--czrs0t xn--czru2d
xn--czru2d xn--d1acj3b xn--d1acj3b xn--d1alf xn--d1alf xn--e1a4c xn--e1a4c xn--eckvdtc9d xn--
eckvdtc9d xn--efvy88h xn--efvy88h xn--fct429k xn--fct429k xn--fhbei xn--fhbei xn--fiq228c5hs xn--
fiq228c5hs xn--fiq64b xn--fiq64b xn--fiqs8s xn--fiqs8s xn--fiqz9s xn--fiqz9s xn--fjq720a xn--fjq720a
xn--flw351e xn--flw351e xn--fpcrj9c3d xn--fpcrj9c3d xn--fzc2c9e2c xn--fzc2c9e2c xn--fzys8d69uvgm xn
--fzys8d69uvgm xn--g2xx48c xn--g2xx48c xn--gckr3f0f xn--gckr3f0f xn--gecrj9c xn--gecrj9c xn--gk3at1e
xn--gk3at1e xn--h2breg3eve xn--h2breg3eve xn--h2brj9c xn--h2brj9c xn--h2brj9c8c xn--h2brj9c8c xn--
hxt814e xn--hxt814e xn--i1b6b1a6a2e xn--i1b6b1a6a2e xn--imr513n xn--imr513n xn--io0a7i xn--io0a7i xn
--j1aef xn--j1aef xn--j1amh xn--j1amh xn--j6w193g xn--j6w193g xn--jlq480n2rg xn--jlq480n2rg xn--
jvr189m xn--jvr189m xn--kcrx77d1x4a xn--kcrx77d1x4a xn--kprw13d xn--kprw13d xn--kpry57d xn--kpry57d
xn--kput3i xn--kput3i xn--l1acc xn--l1acc xn--lgbbat1ad8j xn--lgbbat1ad8j xn--mgb2ddes xn--mgb2ddes
xn--mgb9awbf xn--mgb9awbf xn--mgba3a3ejt xn--mgba3a3ejt xn--mgba3a4f16a xn--mgba3a4f16a xn--
mgba3a4fra xn--mgba3a4fra xn--mgba7c0bbn0a xn--mgba7c0bbn0a xn--mgbaam7a8h xn--mgbaam7a8h xn--
mgbab2bd xn--mgbab2bd xn--mgbah1a3hjkrd xn--mgbah1a3hjkrd xn--mgbai9a5eva00b xn--mgbai9a5eva00b xn--
mgbai9azgqp6j xn--mgbai9azgqp6j xn--mgbayh7gpa xn--mgbayh7gpa xn--mgbbh1a xn--mgbbh1a xn--mgbbh1a71e
xn--mgbbh1a71e xn--mgbc0a9azcg xn--mgbc0a9azcg xn--mgbca7dzdo xn--mgbca7dzdo xn--mgbcpq6gpa1a xn--
mgbcpq6gpa1a xn--mgberp4a5d4a87g xn--mgberp4a5d4a87g xn--mgberp4a5d4ar xn--mgberp4a5d4ar xn--
mgbgu82a xn--mgbgu82a xn--mgbi4ecexp xn--mgbi4ecexp xn--mgbpl2fh xn--mgbpl2fh xn--mgbqly7c0a67fbc xn
--mgbqly7c0a67fbc xn--mgbqly7cvafr xn--mgbqly7cvafr xn--mgbt3dhd xn--mgbt3dhd xn--mgbtf8fl xn--
mgbtf8fl xn--mgbtx2b xn--mgbtx2b xn--mgbx4cd0ab xn--mgbx4cd0ab xn--mix082f xn--mix082f xn--mix891f
xn--mix891f xn--mk1bu44c xn--mk1bu44c xn--mxtq1m xn--mxtq1m xn--ngbc5azd xn--ngbc5azd xn--ngbe9e0a
xn--ngbe9e0a xn--ngbrx xn--ngbrx xn--nnx388a xn--nnx388a xn--node xn--node xn--nqv7f xn--nqv7f xn--
nqv7fs00ema xn--nqv7fs00ema xn--nyqy26a xn--nyqy26a xn--o3cw4h xn--o3cw4h xn--ogbpf8fl xn--ogbpf8fl
xn--otu796d xn--otu796d xn--p1acf xn--p1acf xn--p1ai xn--p1ai xn--pgbs0dh xn--pgbs0dh xn--pssy2u xn
--pssy2u xn--q7ce6a xn--q7ce6a xn--q9jyb4c xn--q9jyb4c xn--qcka1pmc xn--qcka1pmc xn--qxa6a xn--qxa6a
xn--qxam xn--qxam xn--rhqv96g xn--rhqv96g xn--rovu88b xn--rovu88b xn--rvc1e0am3e xn--rvc1e0am3e xn--
s9brj9c xn--s9brj9c xn--ses554g xn--ses554g xn--t60b56a xn--t60b56a xn--tckwe xn--tckwe xn--
tiq49xqyj xn--tiq49xqyj xn--unup4y xn--unup4y xn--vermgensberater-ctb xn--vermgensberater-ctb xn--
vermgensberatung-pwb xn--vermgensberatung-pwb xn--vhquv xn--vhquv xn--vuq861b xn--vuq861b xn--
w4r85el8fhu5dnra xn--w4r85el8fhu5dnra xn--w4rs40l xn--w4rs40l xn--wgbh1c xn--wgbh1c xn--wgbl6a xn--
wgbl6a xn--xhq521b xn--xhq521b xn--xkc2al3hye2a xn--xkc2al3hye2a xn--xkc2dl3a5ee0h xn--xkc2dl3a5ee0h
xn--y9a3aq xn--y9a3aq xn--yfro4i67o xn--yfro4i67o xn--ygbi2ammx xn--ygbi2ammx xn--zfr164b xn--
zfr164b xxx xyz yachts yahoo yamaxun yandex ye yodobashi yoga yokohama you youtube yt yun zappos
zara zero zip zm zone zuerich zw
`

// These suffixes have no recognized top-level rule in the pinned list.
const researchSpecialSuffixes = `*.ck *.er *.fk *.jm *.mm *.np *.pg ac.za agric.za alt.za co.za edu.za gov.za grondar.za law.za mil.za net.za ngo.za nic.za nis.za nom.za org.za school.za tm.za web.za`

var researchTLDSet = func() map[string]struct{} {
	result := make(map[string]struct{})
	for _, tld := range strings.Fields(researchTLDs) {
		result[tld] = struct{}{}
	}
	return result
}()

func validResearchSuffix(host string) bool {
	if _, ok := researchTLDSet[host[strings.LastIndexByte(host, '.')+1:]]; ok {
		return true
	}
	for _, suffix := range strings.Fields(researchSpecialSuffixes) {
		if strings.HasPrefix(suffix, "*.") {
			if strings.HasSuffix(host, suffix[1:]) {
				return true
			}
		} else if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// hostnameProfile converts internationalized hostnames the way the WHATWG URL
// parser does (UTS #46 non-transitional processing without STD3 rules), so a
// Unicode domain reaches DataForSEO in the punycode form OpenSEO sent.
var hostnameProfile = idna.New(
	idna.MapForLookup(),
	idna.Transitional(false),
	idna.BidiRule(),
	idna.CheckJoiners(true),
	idna.StrictDomainName(false),
)

// asciiHostname returns host unchanged when it is ASCII and its punycode form
// otherwise.
func asciiHostname(host string) (string, error) {
	for i := 0; i < len(host); i++ {
		if host[i] >= utf8.RuneSelf {
			return hostnameProfile.ToASCII(host)
		}
	}
	return host, nil
}
