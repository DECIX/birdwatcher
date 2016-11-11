package bird

import (
	"regexp"
	"strconv"
	"strings"
)

type Parsed map[string]interface{}

func emptyLine(line string) bool {
	return len(strings.TrimSpace(line)) == 0
}

func getLinesUnfiltered(input string) []string {
	line_sep := regexp.MustCompile(`((\r?\n)|(\r\n?))`)
	return line_sep.Split(input, -1)
}

func getLinesFromString(input string) []string {
	lines := getLinesUnfiltered(input)

	var filtered []string

	for _, line := range lines {
		if !emptyLine(line) {
			filtered = append(filtered, line)
		}
	}

	return filtered
}

func getLines(input []byte) []string {
	return getLinesFromString(string(input))
}

func specialLine(line string) bool {
	return (strings.HasPrefix(line, "BIRD") ||
		strings.HasPrefix(line, "Access restricted"))
}

func parseStatus(input []byte) Parsed {
	res := Parsed{}
	lines := getLines(input)

	start_line_rx := regexp.MustCompile(`^BIRD\s([0-9\.]+)\s*$`)
	router_id_rx := regexp.MustCompile(`^Router\sID\sis\s([0-9\.]+)\s*$`)
	current_server_rx := regexp.MustCompile(`^Current\sserver\stime\sis\s([0-9\-]+)\s([0-9\:]+)\s*$`)
	last_reboot_rx := regexp.MustCompile(`^Last\sreboot\son\s([0-9\-]+)\s([0-9\:]+)\s*$`)
	last_reconfig_rx := regexp.MustCompile(`^Last\sreconfiguration\son\s([0-9\-]+)\s([0-9\:]+)\s*$`)

	for _, line := range lines {
		if start_line_rx.MatchString(line) {
			res["version"] = start_line_rx.FindStringSubmatch(line)[1]
		} else if router_id_rx.MatchString(line) {
			res["router_id"] = router_id_rx.FindStringSubmatch(line)[1]
		} else if current_server_rx.MatchString(line) {
			res["current_server"] = current_server_rx.FindStringSubmatch(line)[1]
		} else if last_reboot_rx.MatchString(line) {
			res["last_reboot"] = last_reboot_rx.FindStringSubmatch(line)[1]
		} else if last_reconfig_rx.MatchString(line) {
			res["last_reconfig"] = last_reconfig_rx.FindStringSubmatch(line)[1]
		} else {
			res["message"] = line
		}
	}
	return res
}

func parseProtocols(input []byte) Parsed {
	res := Parsed{}
	protocols := []string{}
	lines := getLinesUnfiltered(string(input))

	proto := ""
	for _, line := range lines {
		if emptyLine(line) {
			if !emptyLine(proto) {
				protocols = append(protocols, proto)
			}
			proto = ""
		} else {
			proto += (line + "\n")
		}
	}

	res["protocols"] = protocols
	return res
}

func parseSymbols(input []byte) Parsed {
	res := Parsed{}
	lines := getLines(input)

	key_rx := regexp.MustCompile(`^([^\s]+)\s+(.+)\s*$`)
	for _, line := range lines {
		if specialLine(line) {
			continue
		}

		if key_rx.MatchString(line) {
			groups := key_rx.FindStringSubmatch(line)
			res[groups[2]] = groups[1]
		}
	}

	return res
}

func mainRouteDetail(groups []string, route Parsed) Parsed {
	route["network"] = groups[1]
	route["gateway"] = groups[2]
	route["interface"] = groups[3]
	route["from_protocol"] = groups[4]
	route["age"] = groups[5]
	route["learnt_from"] = groups[6]
	route["primary"] = groups[7] == "*"
	if val, err := strconv.ParseInt(groups[8], 10, 64); err != nil {
		route["metric"] = 0
	} else {
		route["metric"] = val
	}
	return route
}

func parseRoutes(input []byte) Parsed {
	res := Parsed{}
	lines := getLines(input)

	routes := []Parsed{}

	route := Parsed{}
	start_def_rx := regexp.MustCompile(`^([0-9a-f\.\:\/]+)\s+via\s+([0-9a-f\.\:]+)\s+on\s+(\w+)\s+\[(\w+)\s+([0-9\-\:]+)(?:\s+from\s+([0-9a-f\.\:\/]+)){0,1}\]\s+(?:(\*)\s+){0,1}\((\d+)(?:\/\d+){0,1}\).*$`)
	second_rx := regexp.MustCompile(`^\s+via\s+([0-9a-f\.\:]+)\s+on\s+(\w+)\s+\[(\w+)\s+([0-9\-\:]+)(?:\s+from\s+([0-9a-f\.\:\/]+)){0,1}\]\s+(?:(\*)\s+){0,1}\((\d+)(?:\/\d+){0,1}\).*$`)
	type_rx := regexp.MustCompile(`^\s+Type:\s+(.*)\s*$`)
	bgp_rx := regexp.MustCompile(`^\s+BGP.(\w+):\s+(\w+)\s*$`)
	community_rx := regexp.MustCompile(`^\((\d+),(\d+)\)`)
	for _, line := range lines {
		if specialLine(line) || (len(route) == 0 && emptyLine(line)) {
			continue
		}

		if start_def_rx.MatchString(line) {
			if len(route) > 0 {
				routes = append(routes, route)
				route = Parsed{}
			}
			route = mainRouteDetail(start_def_rx.FindStringSubmatch(line), route)
		} else if second_rx.MatchString(line) {
			routes = append(routes, route)
			var network string
			if tmp, ok := route["network"]; ok {
				if val, ok := tmp.(string); ok {
					network = val
				} else {
					continue
				}
			} else {
				continue
			}
			route = Parsed{}

			groups := second_rx.FindStringSubmatch(line)
			first, groups := groups[0], groups[1:]
			groups = append([]string{network}, groups...)
			groups = append([]string{first}, groups...)
			route = mainRouteDetail(groups, route)
		} else if type_rx.MatchString(line) {
			route["type"] = strings.Split(type_rx.FindStringSubmatch(line)[1],
				" ")
		} else if bgp_rx.MatchString(line) {
			groups := bgp_rx.FindStringSubmatch(line)
			bgp := Parsed{}

			if tmp, ok := route["bgp"]; ok {
				if val, ok := tmp.(Parsed); ok {
					bgp = val
				}
			}

			if groups[1] == "community" {
				communities := [][]int64{}
				for _, community := range strings.Split(groups[2], " ") {
					if community_rx.MatchString(community) {
						com_groups := community_rx.FindStringSubmatch(community)
						maj, err := strconv.ParseInt(com_groups[1], 10, 64)
						if err != nil {
							continue
						}
						min, err := strconv.ParseInt(com_groups[2], 10, 64)
						if err != nil {
							continue
						}
						communities = append(communities, []int64{maj, min})
					}
					bgp["communities"] = communities
				}
			} else {
				bgp[groups[1]] = groups[2]
			}

			route["bgp"] = bgp
		}
	}

	if len(route) > 0 {
		routes = append(routes, route)
	}

	res["routes"] = routes
	return res
}

func parseRoutesCount(input []byte) Parsed {
	res := Parsed{}
	lines := getLines(input)

	count_rx := regexp.MustCompile(`^(\d+)\s+of\s+(\d+)\s+routes.*$`)
	for _, line := range lines {
		if specialLine(line) {
			continue
		}

		if count_rx.MatchString(line) {
			i, err := strconv.ParseInt(count_rx.FindStringSubmatch(line)[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			res["routes"] = i
		}
	}

	return res
}

// Will snake_case a value like that:
// I am a Weird stRiNg -> i_am_a_weird_string
func treatKey(key string) string {
	spaces := regexp.MustCompile(`\s+`)
	key = spaces.ReplaceAllString(key, "_")
	return strings.ToLower(key)
}

func parseBgp(input string) Parsed {
	res := Parsed{}
	lines := getLinesFromString(input)
	route_changes := Parsed{}

	bgp_rx := regexp.MustCompile(`^([\w\.]+)\s+BGP\s+(\w+)\s+(\w+)\s+([0-9]{4}-[0-9]{2}-[0-9]{2}\s+[0-9]{2}:[0-9]{2}:[0-9]{2})\s*(\w+)?.*$`)
	num_val_rx := regexp.MustCompile(`^\s+([^:]+):\s+([\d]+)\s*$`)
	str_val_rx := regexp.MustCompile(`^\s+([^:]+):\s+(.+)\s*$`)
	routes_rx := regexp.MustCompile(`^\s+Routes:\s+(\d+)\s+imported,\s+(\d+)\s+filtered,\s+(\d+)\s+exported,\s+(\d+)\s+preferred\s*$`)
	imp_updates_rx := regexp.MustCompile(`^\s+Import updates:\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*$`)
	imp_withdraws_rx := regexp.MustCompile(`^\s+Import withdraws:\s+(\d+)\s+(\d+)\s+\-\-\-\s+(\d+)\s+(\d+)\s*$`)
	exp_updates_rx := regexp.MustCompile(`^\s+Export updates:\s+(\d+)\s+(\d+)\s+(\d+)\s+\-\-\-\s+(\d+)\s*$`)
	exp_withdraws_rx := regexp.MustCompile(`^\s+Export withdraws:\s+(\d+)(\s+\-\-\-)+\s+(\d+)\s*$`)
	for _, line := range lines {
		if bgp_rx.MatchString(line) {
			groups := bgp_rx.FindStringSubmatch(line)

			res["protocol"] = groups[1]
			res["bird_protocol"] = "BGP"
			res["table"] = groups[2]
			res["state"] = groups[3]
			res["state_changed"] = groups[4]
			res["connection"] = groups[5]
		} else if routes_rx.MatchString(line) {
			routes := Parsed{}
			groups := routes_rx.FindStringSubmatch(line)

			imported, err := strconv.ParseInt(groups[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			filtered, err := strconv.ParseInt(groups[2], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			exported, err := strconv.ParseInt(groups[3], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			preferred, err := strconv.ParseInt(groups[4], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}

			routes["imported"] = imported
			routes["filtered"] = filtered
			routes["exported"] = exported
			routes["preferred"] = preferred

			res["routes"] = routes
		} else if imp_updates_rx.MatchString(line) {
			updates := Parsed{}
			groups := imp_updates_rx.FindStringSubmatch(line)

			received, err := strconv.ParseInt(groups[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			rejected, err := strconv.ParseInt(groups[2], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			filtered, err := strconv.ParseInt(groups[3], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			ignored, err := strconv.ParseInt(groups[4], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			accepted, err := strconv.ParseInt(groups[5], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}

			updates["received"] = received
			updates["rejected"] = rejected
			updates["filtered"] = filtered
			updates["ignored"] = ignored
			updates["accepted"] = accepted

			route_changes["import_updates"] = updates
		} else if imp_withdraws_rx.MatchString(line) {
			updates := Parsed{}
			groups := imp_withdraws_rx.FindStringSubmatch(line)

			received, err := strconv.ParseInt(groups[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			rejected, err := strconv.ParseInt(groups[2], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			filtered, err := strconv.ParseInt(groups[3], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			accepted, err := strconv.ParseInt(groups[4], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}

			updates["received"] = received
			updates["rejected"] = rejected
			updates["filtered"] = filtered
			updates["accepted"] = accepted

			route_changes["import_withdraws"] = updates
		} else if exp_updates_rx.MatchString(line) {
			updates := Parsed{}
			groups := exp_updates_rx.FindStringSubmatch(line)

			received, err := strconv.ParseInt(groups[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			rejected, err := strconv.ParseInt(groups[2], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			ignored, err := strconv.ParseInt(groups[3], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			accepted, err := strconv.ParseInt(groups[4], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}

			updates["received"] = received
			updates["rejected"] = rejected
			updates["ignored"] = ignored
			updates["accepted"] = accepted

			route_changes["export_updates"] = updates
		} else if exp_withdraws_rx.MatchString(line) {
			updates := Parsed{}
			groups := exp_withdraws_rx.FindStringSubmatch(line)

			received, err := strconv.ParseInt(groups[1], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}
			accepted, err := strconv.ParseInt(groups[3], 10, 64)
			if err != nil {
				// ignore for now
				continue
			}

			updates["received"] = received
			updates["accepted"] = accepted

			route_changes["export_withdraws"] = updates
		} else if num_val_rx.MatchString(line) {
			groups := num_val_rx.FindStringSubmatch(line)

			key := treatKey(groups[1])
			val, err := strconv.ParseInt(groups[2], 10, 64)

			if err != nil {
				// ignore for now
				continue
			}

			res[key] = val
		} else if str_val_rx.MatchString(line) {
			groups := str_val_rx.FindStringSubmatch(line)

			key := treatKey(groups[1])

			res[key] = groups[2]
		}
	}

	res["route_changes"] = route_changes

	if _, ok := res["routes"]; !ok {
		routes := Parsed{}

		routes["accepted"] = 0
		routes["filtered"] = 0
		routes["exported"] = 0
		routes["preferred"] = 0

		res["routes"] = routes
	}

	return res
}