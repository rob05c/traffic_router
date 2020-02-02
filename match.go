package main

import (
	"errors"
	"regexp"
	"strings"
)

// TODO Traffic Monitor has to do this same matching to determine stat DSes. Put match logic in a generic location, and use with both TR and TM.

func NewDNSDSMatch(matchStr string) (DNSDSMatch, error) {
	if strings.HasPrefix(matchStr, `.*\.`) && strings.HasSuffix(matchStr, `\..*`) {
		return dnsDSMatchContains{str: strings.TrimSuffix(strings.TrimPrefix(matchStr, `.*\.`), `\..*`)}, nil
	} else if ValidFQDN(matchStr) {
		// If the match string is a valid FQDN, we assume it's not a regex.
		// Be aware it could still be a regex, and e.g. 'foo.bar.com' could be actually wanting to match those dots as anything, e.g. match 'fooabar.com'.
		// But that would be very strange.
		return dnsDSMatchLiteral{str: matchStr}, nil
	} else {
		re, err := regexp.Compile(matchStr)
		if err != nil {
			return nil, errors.New("compiling regex: " + err.Error())
		}
		return dnsDSMatchRegex{re: re}, nil
	}
}

type DNSDSMatch interface {
	Match(fqdn string) bool
}

type dnsDSMatchContains struct {
	str string
}

func (dm dnsDSMatchContains) Match(fqdn string) bool { return strings.Contains(fqdn, dm.str) }

type dnsDSMatchLiteral struct {
	str string
}

func (dm dnsDSMatchLiteral) Match(fqdn string) bool { return fqdn == dm.str }

type dnsDSMatchRegex struct {
	re *regexp.Regexp
}

func (dm dnsDSMatchRegex) Match(fqdn string) bool { return dm.re.MatchString(fqdn) }
