# No-expensive-dev policy: warn when a resource whose name contains
# "dev" or "staging" costs more than $100/mo. Forces a conversation
# about whether non-prod really needs that hardware.

package c3x

warn[msg] {
    resource := input.estimate.resources[_]
    is_non_prod(resource.name)
    resource.monthly_cost > 100
    msg := sprintf("non-prod resource %v.%v costs %v %v/mo — review sizing",
                   [resource.kind, resource.name, input.estimate.currency, resource.monthly_cost])
}

is_non_prod(name) {
    contains(lower(name), "dev")
}

is_non_prod(name) {
    contains(lower(name), "staging")
}

is_non_prod(name) {
    contains(lower(name), "stg")
}
