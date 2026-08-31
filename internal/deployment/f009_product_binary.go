package deployment

func init() {
	const name = "stagecore"
	for _, existing := range RequiredBinaries {
		if existing == name {
			return
		}
	}
	RequiredBinaries = append(RequiredBinaries, name)
}
