package plugin

// Official team-site assets, checked September 2026. City-only guide names
// resolve only inside CEBL, so Calgary never borrows a hockey identity.
var ceblTeamIdentities = []struct {
	names []string
	logo  string
}{
	{[]string{"brampton", "brampton honey badgers"}, "https://irp.cdn-website.com/ffc1e51d/dms3rep/multi/opt/BramptonHoneyBadgers_Icon+%282%29-1920w.png"},
	{[]string{"calgary", "calgary surge"}, "https://irp.cdn-website.com/d8d53c44/dms3rep/multi/Surge-200x200.png"},
	{[]string{"edmonton", "edmonton stingers"}, "https://irp.cdn-website.com/a0839893/dms3rep/multi/EdmontonStingers_Icon+%281%29.png"},
	{[]string{"montreal", "montreal alliance", "montréal", "montréal alliance"}, "https://irp.cdn-website.com/ed6f4f65/dms3rep/multi/MontrealAlliance_Icon+%281%29.png"},
	{[]string{"niagara", "niagara river lions"}, "https://irp.cdn-website.com/082843e8/dms3rep/multi/Niagara_Full_Green_Wordmark+%281%29.png"},
	{[]string{"ottawa", "ottawa blackjacks", "ottawa black jacks"}, "https://irp.cdn-website.com/ef39452c/dms3rep/multi/OttawaBlackJacks_Primary_White-RedWordmark-18d6c3a3.png"},
	{[]string{"saskatoon", "saskatchewan", "saskatoon mamba", "saskatchewan mamba"}, "https://irp.cdn-website.com/6d469176/dms3rep/multi/MambaIcon_RapidPath_57x57.png"},
	{[]string{"scarborough", "scarborough shooting stars"}, "https://irp.cdn-website.com/be96ae86/dms3rep/multi/scarborough-shooting-stars-logo-cebl-team-500.png"},
	{[]string{"vancouver", "vancouver bandits"}, "https://irp.cdn-website.com/d8d53c44/dms3rep/multi/VanBandits_blk_org.png"},
	{[]string{"winnipeg", "winnipeg sea bears", "winnipeg seabears"}, "https://irp.cdn-website.com/8d6ff7b6/dms3rep/multi/WinnipegSeaBears__Icon.png"},
}

func ceblTeamLogo(name string) string {
	name = normalizeSportsIdentityText(name)
	for _, team := range ceblTeamIdentities {
		for _, alias := range team.names {
			if name == alias {
				return team.logo
			}
		}
	}
	return ""
}

func isCEBLTeamLogo(value string) bool {
	for _, team := range ceblTeamIdentities {
		if value == team.logo {
			return true
		}
	}
	return false
}
