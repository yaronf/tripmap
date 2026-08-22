package httpserver

import (
	"fmt"
	"net/http"
)

const legalEffectiveDate = "2026-08-22"

func (s *Server) contactEmail() string {
	if e := s.cfg.ContactEmail; e != "" {
		return e
	}
	return "yaronf@gmx.com"
}

func (s *Server) handlePrivacy(w http.ResponseWriter, _ *http.Request) {
	email := htmlEscape(s.contactEmail())
	body := fmt.Sprintf(`
<p>Tripmap (<code>%s</code>) is a private, invitation-only service for viewing road-trip itineraries.</p>
<h2>Information we collect</h2>
<ul>
<li><strong>Sign-in.</strong> If you sign in with Google, we receive your email address and display name through our authentication provider (Descope). We use this only to verify that you are on the trip operator’s allowlist and to show who is signed in.</li>
<li><strong>Session.</strong> We set an HTTP-only session cookie so you stay signed in between visits.</li>
<li><strong>Trip content.</strong> Itineraries, shared notes, and comments you can view or edit are stored in the operator’s cloud storage (Amazon Web Services in the EU).</li>
<li><strong>Chat.</strong> If in-viewer chat is enabled for your account, your messages may be sent to OpenAI to generate replies. Do not enter sensitive personal data in chat.</li>
</ul>
<h2>How we use information</h2>
<p>We use the above solely to operate Tripmap for invited travelers: authentication, displaying itineraries, shared notes, comments, and optional chat. We do not sell your personal information.</p>
<h2>Third parties</h2>
<ul>
<li>Google — OAuth sign-in (subject to <a href="https://policies.google.com/privacy">Google’s Privacy Policy</a>)</li>
<li>Descope — authentication processing</li>
<li>Amazon Web Services — hosting and storage</li>
<li>OpenAI — optional chat responses when enabled</li>
</ul>
<h2>Retention and security</h2>
<p>Session cookies expire after a limited period of inactivity. Trip data is kept for the duration of the trip season unless the operator deletes it. We use industry-standard transport encryption (HTTPS).</p>
<h2>Your choices</h2>
<p>Access is optional and by invitation. You may stop using the service at any time and sign out via the site. To request deletion of comments you posted, contact us.</p>
<h2>Contact</h2>
<p>Questions about this policy: <a href="mailto:%s">%s</a>.</p>
<p class="muted">Effective %s.</p>`,
		htmlEscape(s.baseURLFromConfig()), email, email, legalEffectiveDate)
	s.writeSitePage(w, "Privacy Policy — "+siteAppName, body)
}

func (s *Server) handleTerms(w http.ResponseWriter, _ *http.Request) {
	email := htmlEscape(s.contactEmail())
	body := fmt.Sprintf(`
<p>These Terms of Service (“Terms”) apply to your use of Tripmap at <code>%s</code>.</p>
<h2>The service</h2>
<p>Tripmap provides private itinerary viewers and related tools for invited travelers on a specific trip. The service may run seasonally and may be unavailable when not in season.</p>
<h2>Eligibility and access</h2>
<p>Access is by invitation only. The trip operator maintains an email allowlist. You must sign in with a Google account whose email address is on that list. We may revoke access at any time.</p>
<h2>Acceptable use</h2>
<ul>
<li>Use Tripmap only for the intended trip and shared planning.</li>
<li>Do not attempt to bypass authentication, scrape the service, or interfere with its operation.</li>
<li>Do not post unlawful, harassing, or malicious content in notes, comments, or chat.</li>
</ul>
<h2>Content</h2>
<p>Itineraries and operator-provided content remain the property of the trip operator. You retain responsibility for comments and notes you submit. The operator may remove content at their discretion.</p>
<h2>Disclaimer</h2>
<p>Tripmap is provided “as is” without warranties. Route estimates, maps, and AI chat replies may be inaccurate. You are responsible for your own travel decisions and safety.</p>
<h2>Limitation of liability</h2>
<p>To the fullest extent permitted by law, the operator is not liable for indirect or consequential damages arising from use of Tripmap.</p>
<h2>Changes</h2>
<p>We may update these Terms or discontinue the service. Continued use after changes constitutes acceptance of the updated Terms.</p>
<h2>Contact</h2>
<p>Questions about these Terms: <a href="mailto:%s">%s</a>. See also our <a href="/privacy">Privacy Policy</a>.</p>
<p class="muted">Effective %s.</p>`,
		htmlEscape(s.baseURLFromConfig()), email, email, legalEffectiveDate)
	s.writeSitePage(w, "Terms of Service — "+siteAppName, body)
}

func (s *Server) baseURLFromConfig() string {
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL
	}
	return "https://tripmap.sheffer.org"
}

func siteFooterHTML() string {
	return `<footer><p class="muted"><a href="/">Home</a> · <a href="/privacy">Privacy Policy</a> · <a href="/terms">Terms of Service</a></p></footer>`
}

func (s *Server) writeSitePage(w http.ResponseWriter, title, bodyHTML string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	extraHead := s.siteExtraHead()
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>%s</title>
%s<link rel="icon" href="/favicon.png" type="image/png"/>
<style>%s</style>
</head><body>
<header><p class="brand"><a href="/">%s</a></p></header>
<main>%s</main>
%s
</body></html>`, htmlEscape(title), extraHead, sitePageCSS, siteAppName, bodyHTML, siteFooterHTML())
}

const sitePageCSS = `body{font-family:system-ui,sans-serif;max-width:42rem;margin:0 auto;padding:0 1rem 2.5rem;line-height:1.55;color:#1a1f1c;background:#f3efe6}
a{color:#0f5c5c}code{font-size:.9em}.muted{color:#5c6560;font-size:.9rem}
header{padding:1.75rem 0 .5rem;border-bottom:1px solid #ddd6c8;margin-bottom:1.25rem}
.brand{margin:0;font-size:1.15rem;font-weight:600}.brand a{text-decoration:none;color:#0f5c5c}
main h1{font-size:1.75rem;margin:0 0 .35rem}.tagline{font-size:1.05rem;color:#5c6560;margin:0 0 1.25rem}
main h2{font-size:1.05rem;margin:1.5rem 0 .5rem}
main ul{padding-left:1.25rem}main li{margin:.35rem 0}
footer{margin-top:2rem;padding-top:1rem;border-top:1px solid #ddd6c8}
.btn{display:inline-block;padding:.65rem 1.1rem;background:#0f5c5c;color:#fff;text-decoration:none;border-radius:6px;font-weight:600}
.btn:hover{background:#0d4f4f}
ul.trips{list-style:none;padding:0;margin:1.25rem 0}ul.trips li{margin:.55rem 0;padding:.55rem 0;border-top:1px solid #ddd6c8}
ul.trips li:first-child{border-top:0}ul.trips a{font-weight:600;text-decoration:none}ul.trips a:hover{text-decoration:underline}
ul.trips .id{display:block;font-size:.85rem;color:#5c6560;font-weight:400}`
