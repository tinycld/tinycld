// Shapes a package's membership table must satisfy for the group share UI.
// A package's row type (e.g. BoardsProjectMembers) is structurally assignable
// because it carries these four fields; the rest of its fields are ignored.

export interface GroupGrantRow<Role extends string = string> {
    id: string
    user: string
    group: string
    role: Role
}

// A grant row as core builds it. The package's `buildRow` adds its own fields
// (the resource id, created_by, …) and returns the collection's insert type.
export interface NewGroupGrant<Role extends string> {
    id: string
    user: ''
    group: string
    role: Role
}

export interface GroupRoleOption<Role extends string> {
    value: Role
    label: string
}

export interface GroupGrant<Role extends string> {
    grantId: string
    groupId: string
    role: Role
}
