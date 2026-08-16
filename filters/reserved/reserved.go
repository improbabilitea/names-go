// Package reserved provides a default list of terms a product should keep for
// itself.
//
//	names.WithReservedWords(reserved.Default)
//
// Matching ignores case and separators, so "support" also blocks the slug
// "sup-port" and the display name "Sup Port".
package reserved

// Default covers the terms that cause trouble in almost any product: they
// impersonate the operator, collide with routing, or read as an official
// account.
//
// It is a starting point, not a policy. Extend it with whatever a specific
// product routes or reserves — WithReservedWords accumulates across calls, so
// there is no need to copy this slice to add to it:
//
//	names.WithReservedWords(reserved.Default),
//	names.WithReservedWords([]string{"checkout", "invoices"}),
var Default = []string{
	// Operator and staff impersonation. The expensive failure mode: a user
	// named "support" is a phishing vector, not a naming collision.
	"admin",
	"administrator",
	"billing",
	"root",
	"staff",
	"support",
	"superuser",
	"sysadmin",
	"system",
	"moderator",
	"official",
	"security",
	"team",
	"helpdesk",
	"webmaster",
	"postmaster",
	"hostmaster",
	"abuse",
	"noreply",
	"donotreply",

	// Route collisions. A name that is also a path segment breaks whichever of
	// the two was added second.
	"about",
	"account",
	"accounts",
	"api",
	"assets",
	"auth",
	"blog",
	"callback",
	"careers",
	"cdn",
	"contact",
	"dashboard",
	"docs",
	"download",
	"downloads",
	"explore",
	"faq",
	"favicon",
	"feed",
	"files",
	"health",
	"help",
	"home",
	"images",
	"index",
	"internal",
	"invite",
	"legal",
	"login",
	"logout",
	"me",
	"media",
	"metrics",
	"new",
	"news",
	"notifications",
	"oauth",
	"orders",
	"pricing",
	"privacy",
	"profile",
	"public",
	"register",
	"reset",
	"robots",
	"rss",
	"search",
	"service",
	"services",
	"session",
	"settings",
	"signin",
	"signup",
	"sitemap",
	"static",
	"status",
	"terms",
	"test",
	"upload",
	"user",
	"users",
	"webhook",
	"webhooks",

	// Reserved-looking words that read as a claim about the account.
	"everyone",
	"here",
	"all",
	"null",
	"undefined",
	"anonymous",
	"deleted",
	"unknown",
}
