package rbac

import data.roles_scopes as roles_scopes
import data.roles_attributes as roles_attributes
import data.rolecollection as rolecollection

default allow = false

allow {
    has_scope
}

allow {
    has_attribute
}

allow {
    has_role
}

has_role {
    io.jwt.decode(input.token, [header, payload, sig])
    JWTrolecollections := payload["xs.system.attributes"]["xs.rolecollections"][_]
    rolecollection[JWTrolecollections][_] == input.role
}

has_scope {
    io.jwt.decode(input.token, [header, payload, sig])
    JWTrolecollections := payload["xs.system.attributes"]["xs.rolecollections"][_]
    roles_scopes[rolecollection[JWTrolecollections][_]][_] == input.scope
}

has_attribute {
    io.jwt.decode(input.token, [header, payload, sig])
    JWTrolecollections := payload["xs.system.attributes"]["xs.rolecollections"][_]
    attribute = roles_attributes[rolecollection[JWTrolecollections][_]][input.attributeName]
    attribute[_] == input.attributeValue
}