package coreserver

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

// The server fetches from, and PUTs to, a URL the caller supplies. That is the
// feature — an operator's own bucket is the only place a backup can go — but it
// also makes the server a fetcher for whoever can reach this endpoint.
//
// Only the org owner (restore) or an admin (backup) can reach it, and no response
// body is ever surfaced to the caller: a backup discards the target's response,
// and a restore's body must decrypt with the caller's own passphrase or the
// restore fails. The ledger records hostnames only. So the residual risk is a
// blind request from the server's network position, made by someone who already
// administers the deployment.
//
// What is still worth refusing is the class of address that is only reachable
// from the server and is never a legitimate backup target: the loopback
// interface, link-local (169.254.0.0/16 and fe80::/10, which is where every
// cloud provider's instance-metadata service lives), the unspecified address,
// and multicast. RFC1918 private ranges stay ALLOWED — a MinIO box on the
// operator's own LAN is a normal target.
const allowLoopbackEnv = "TINYCLD_BACKUP_ALLOW_LOOPBACK"

// allowLoopbackTargets is read once at registration. A test flips it through
// refreshBackupTargetPolicy after setting the variable.
var allowLoopbackTargets = os.Getenv(allowLoopbackEnv) == "1"

// refreshBackupTargetPolicy re-reads the override. Register calls it so the
// variable is read at registration rather than at package init, which a test
// cannot get ahead of.
func refreshBackupTargetPolicy() {
	allowLoopbackTargets = os.Getenv(allowLoopbackEnv) == "1"
}

const targetNotRoutable = "The target must be a public or private network host, not loopback or link-local."

var errTargetNotRoutable = errors.New(targetNotRoutable)

// checkBackupTarget resolves the URL's host and refuses the addresses that are
// only reachable from the server itself.
//
// It resolves rather than pattern-matching the hostname: "localtest.me" and any
// attacker-controlled name can resolve to 127.0.0.1, so the name says nothing.
// This is a check, not a guarantee — the name can resolve differently when the
// transfer actually runs — which is why it is documented as reducing the surface
// rather than closing it.
func checkBackupTarget(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errTargetNotRoutable
	}
	host := u.Hostname()
	if host == "" {
		return errTargetNotRoutable
	}
	if allowLoopbackTargets {
		return nil
	}
	// A bracketed or bare IP literal needs no resolver.
	if ip := net.ParseIP(host); ip != nil {
		if !isRoutableTarget(ip) {
			return errTargetNotRoutable
		}
		return nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		// A name that does not resolve is refused as unusable rather than passed
		// to the transport, which would fail a moment later with a ledger row
		// nobody can read.
		return errors.New("The target's hostname could not be resolved.")
	}
	for _, ip := range addrs {
		// EVERY address has to be acceptable. A name with one public and one
		// loopback address would otherwise be admitted and then, depending on
		// the resolver's ordering at transfer time, reach the loopback one.
		if !isRoutableTarget(ip) {
			return errTargetNotRoutable
		}
	}
	return nil
}

func isRoutableTarget(ip net.IP) bool {
	switch {
	case ip.IsLoopback(), ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return false
	case ip.IsUnspecified(), ip.IsMulticast(), ip.IsInterfaceLocalMulticast():
		return false
	}
	return true
}

// httpScheme is the one check that has to come first: a target that is not
// http(s) is refused before it is resolved, because "file:///etc/passwd" has no
// host to resolve and would otherwise be reported as unresolvable.
func httpScheme(raw string) bool {
	return strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://")
}
