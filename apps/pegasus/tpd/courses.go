// packages/tpd/courses.go

package tpd

import "pegasus_suite/betting/betmatic"

var CourseCodes = map[string]betmatic.RacingCode{
	"01": betmatic.THOROUGHBRED, // Ascot
	"03": betmatic.THOROUGHBRED, // Bangor-on-Dee
	"04": betmatic.THOROUGHBRED, // Bath
	"06": betmatic.THOROUGHBRED, // Brighton
	"11": betmatic.THOROUGHBRED, // Chepstow
	"12": betmatic.THOROUGHBRED, // Chester
	"14": betmatic.THOROUGHBRED, // Doncaster
	"17": betmatic.THOROUGHBRED, // Fakenham
	"19": betmatic.THOROUGHBRED, // Fontwell Park
	"23": betmatic.THOROUGHBRED, // Hereford
	"24": betmatic.THOROUGHBRED, // Hexham
	"30": betmatic.THOROUGHBRED, // Lingfield Park
	"34": betmatic.THOROUGHBRED, // Newbury
	"35": betmatic.THOROUGHBRED, // Newcastle
	"37": betmatic.THOROUGHBRED, // Newton Abbot
	"40": betmatic.THOROUGHBRED, // Plumpton
	"43": betmatic.THOROUGHBRED, // Ripon
	"46": betmatic.THOROUGHBRED, // Sedgefield
	"47": betmatic.THOROUGHBRED, // Southwell
	"53": betmatic.THOROUGHBRED, // Uttoxeter
	"57": betmatic.THOROUGHBRED, // Windsor
	"58": betmatic.THOROUGHBRED, // Wolverhampton
	"59": betmatic.THOROUGHBRED, // Worcester
	"61": betmatic.THOROUGHBRED, // Yarmouth
	"64": betmatic.THOROUGHBRED, // Ffos Las
	"66": betmatic.THOROUGHBRED, // King Khalid
	"67": betmatic.THOROUGHBRED, // Hippodromo
	"68": betmatic.THOROUGHBRED, // King Abdulaziz
	"69": betmatic.THOROUGHBRED, // Club Hipico Santiago
	"70": betmatic.THOROUGHBRED, // Louisiana Downs
	"71": betmatic.THOROUGHBRED, // Golden Gate Fields
	"73": betmatic.THOROUGHBRED, // Laurel Park
	"74": betmatic.THOROUGHBRED, // Pimlico
	"75": betmatic.THOROUGHBRED, // Keenland
	"76": betmatic.THOROUGHBRED, // Mahoning Valley
	"77": betmatic.THOROUGHBRED, // Penn National
	"78": betmatic.THOROUGHBRED, // Colonial Downs
	"79": betmatic.THOROUGHBRED, // Del Mar
	"80": betmatic.THOROUGHBRED, // Sam Houston
	"81": betmatic.THOROUGHBRED, // Oaklawn Park
	"82": betmatic.THOROUGHBRED, // Canterbury Park
	"83": betmatic.THOROUGHBRED, // Kentucky Downs
	"84": betmatic.THOROUGHBRED, // Monmouth Park
	"85": betmatic.THOROUGHBRED, // Tampa Bay Downs
	"86": betmatic.THOROUGHBRED, // Gulfstream Park
	"87": betmatic.HARNESS,      // Meadowlands
	"88": betmatic.HARNESS,      // Hoosier Park
	"89": betmatic.THOROUGHBRED, // Horseshoe Indianapolis
	"90": betmatic.THOROUGHBRED, // Woodbine
	"91": betmatic.HARNESS,      // Mohawk Park
	"93": betmatic.THOROUGHBRED, // Lone Star Park
	"94": betmatic.THOROUGHBRED, // Santa Anita
	"95": betmatic.THOROUGHBRED, // Turfway Park
	"96": betmatic.THOROUGHBRED, // Aqueduct
	"97": betmatic.THOROUGHBRED, // Churchill Downs
	"98": betmatic.THOROUGHBRED, // Belmont Park
	"HD": betmatic.THOROUGHBRED, // Hyderabad
}
