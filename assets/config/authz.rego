#
# Use https://play.openpolicyagent.org for easier editing/validation of this policy file
#

package example.authz

default allow := false

allow = response if {
    input.method == "GET"

    response := {
        "ok": true
    }
}
