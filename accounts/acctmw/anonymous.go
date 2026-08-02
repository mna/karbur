package acctmw

// TODO: implement anonymous session generation, checks if there is no active
// session, and if so creates one not associated to any account. This means the
// load middleware will load a session id but not an account, so the authorize
// middleware will still work (checks for account). Anonymous session cookie is
// always session-scoped (deleted when browser closed) and should probably have
// a short expiration or at least short idle expiration (30 minutes, 12 hours)?
