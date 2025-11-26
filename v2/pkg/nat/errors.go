package nat

import "errors"

// ErrIPFamilyMismatch is the sentinel error to indicate that FoUTunnel or Egress
// cannot handle the given address because it is not setup for the address family.
var ErrIPFamilyMismatch = errors.New("no matching IP family")
