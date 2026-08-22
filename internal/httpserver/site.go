package httpserver

import (
	"fmt"
	"net/http"
)

const siteAppName = "Tripmap"

const homeAppDescription = "Tripmap is a web application for private multi-day road trip itineraries. Trip organizers publish routes and maps; invited travelers sign in with Hellō to view the itinerary, shared notes, and comments."

func (s *Server) siteExtraHead() string {
	if token := s.cfg.GoogleSiteVerification; token != "" {
		return fmt.Sprintf(`<meta name="google-site-verification" content="%s"/>`, htmlEscape(token))
	}
	return ""
}

func (s *Server) homePageHead() string {
	desc := htmlEscape(homeAppDescription)
	name := htmlEscape(siteAppName)
	return fmt.Sprintf(`<title>%s</title>
<meta name="application-name" content="%s"/>
<meta name="description" content="%s"/>
<meta property="og:site_name" content="%s"/>
<meta property="og:title" content="%s"/>
<meta property="og:description" content="%s"/>
<meta name="apple-mobile-web-app-title" content="%s"/>
%s`, name, name, desc, name, name, desc, name, s.siteExtraHead())
}

func (s *Server) writeHomePage(w http.ResponseWriter, bodyHTML string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
%s<link rel="icon" href="/favicon.png" type="image/png"/>
<link href="https://cdn.hello.coop/css/hello-btn.css" rel="stylesheet"/>
<style>%s</style>
</head><body>
<header><p class="brand">%s</p></header>
<main>%s</main>
%s
</body></html>`, s.homePageHead(), sitePageCSS, siteAppName, bodyHTML, siteFooterHTML())
}

func (s *Server) homePageMain(helloOn, authed bool, sess sessionCookie) string {
	body := fmt.Sprintf(`<h1>%s</h1>
<h2>Purpose of the application</h2>
<p><strong>%s</strong> %s Hellō sign-in is used only to verify invited users and display their name and email while signed in.</p>
<p><a href="/privacy">Privacy Policy</a> · <a href="/terms">Terms of Service</a></p>
<h2>App functionality</h2>
<p>The %s application provides the following features for invited trip participants:</p>
<ul>
<li>Interactive maps and day-by-day driving routes for a private trip</li>
<li>Shared trip notes that travelers can read and edit together</li>
<li>Comments on trip days and stops</li>
<li>Optional in-viewer chat assistant for trip questions (when enabled by the organizer)</li>
</ul>
<h2>Who can use %s</h2>
<p>%s is invitation-only. The trip operator adds your Hellō account (email or subject id) to an allowlist. %s is not a public travel search, booking, or marketplace service.</p>`,
		siteAppName,
		siteAppName,
		homeAppDescription,
		siteAppName,
		siteAppName,
		siteAppName,
		siteAppName)

	if !helloOn {
		body += `<p class="muted">Hellō sign-in is available during the active trip season.</p>`
	} else if authed {
		body += fmt.Sprintf(`
<p>Signed in as <strong>%s</strong> (%s)</p>
<p><a href="/auth/logout">Sign out</a></p>`,
			htmlEscape(sess.Name), htmlEscape(sess.Email))
	} else {
		body += `
<div class="hello-container">
  <button class="hello-btn" type="button" onclick="helloLogin(event)">ō&nbsp;&nbsp;Continue with Hellō</button>
</div>
<script>
function helloLogin(event){
  event.target.classList.add('hello-btn-loader');
  event.target.disabled = true;
  window.location.href = '/auth/hello/login';
}
</script>
<p class="muted">You need an invitation from the trip organizer to sign in.</p>`
	}
	return body
}
