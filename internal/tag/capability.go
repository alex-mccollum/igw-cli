package tag

func (r ImportRequest) verifiedJSON() bool {
	return r.Format == "json" && (r.CollisionPolicy == "Abort" || r.CollisionPolicy == "Overwrite" || r.CollisionPolicy == "MergeOverwrite")
}
