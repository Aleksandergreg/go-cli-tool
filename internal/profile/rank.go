package profile

// rankThresholds span the whole catalog: Linux alone (1,755 XP) reaches
// Staff SRE, and the top rank needs most of the Docker track (3,085 XP in
// total) without requiring a hint-free run. Thresholds up to Senior SRE are
// unchanged from earlier releases, so no existing profile loses its rank.
var rankThresholds = []struct {
	name string
	xp   int
}{
	{name: "Intern", xp: 0},
	{name: "Operator", xp: 100},
	{name: "Junior Sysadmin", xp: 250},
	{name: "Sysadmin", xp: 450},
	{name: "SRE", xp: 650},
	{name: "Senior SRE", xp: 1100},
	{name: "Staff SRE", xp: 1600},
	{name: "Principal SRE", xp: 2150},
	{name: "Distinguished Engineer", xp: 2700},
}

func (p Profile) Level() int { return p.XP/100 + 1 }

func (p Profile) Rank() string {
	for index := len(rankThresholds) - 1; index >= 0; index-- {
		if p.XP >= rankThresholds[index].xp {
			return rankThresholds[index].name
		}
	}
	return rankThresholds[0].name
}

func (p Profile) NextRank() (string, int, bool) {
	for _, threshold := range rankThresholds[1:] {
		if p.XP < threshold.xp {
			return threshold.name, threshold.xp - p.XP, true
		}
	}
	return "", 0, false
}
