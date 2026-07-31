package alt_db

import (
	"github.com/google/uuid"
)

// statsTestUserID is the tenant the §2.M count tests read for.
//
// A fixed non-zero id rather than a user planted on the context, which is what
// these drivers took before ADR-000954 Wave 3 batch 5. The move is the point:
// the count runs inside alt-data-hub now, where the context describes a peer
// certificate naming alt-backend, so the owner has to be an argument or it is
// not the reader's at all.
var statsTestUserID = uuid.MustParse("9f8e7d6c-5b4a-4392-8281-706f5e4d3c2b")
