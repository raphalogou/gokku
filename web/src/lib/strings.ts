// Every user-facing string lives here so a translation can be added later (NFR-6).
// Messages from the API are already user-facing and are shown as they come.

export const strings = {
  product: "Gokku",
  tagline: "A web interface for Dokku",

  common: {
    cancel: "Cancel",
    close: "Close",
    done: "Done",
    save: "Save",
    saving: "Saving…",
    copy: "Copy",
    copied: "Copied",
    retry: "Try again",
    loading: "Loading…",
    never: "Never",
    you: "You",
    moreActions: (name: string) => `More actions for ${name}`,
  },

  errors: {
    unexpected: "Something went wrong. Try again, and check the server log if it keeps happening.",
    notFoundTitle: "Page not found",
    notFoundBody: "This page does not exist. Check the address or go back to your projects.",
    forbiddenTitle: "Not available for your role",
    forbiddenBody: "Ask an admin if you need access to this page.",
    loadFailed: "This page could not be loaded.",
    goHome: "Go to projects",
    signedOut: "You were signed out. Sign in again to continue.",
  },

  roles: {
    viewer: "Viewer",
    operator: "Operator",
    admin: "Admin",
    viewerHint: "Read everything, including logs. Secrets stay masked.",
    operatorHint: "Also restart, deploy, scale, and edit config and domains.",
    adminHint: "Full control, including users, tokens and settings.",
  },

  status: {
    active: "Active",
    invited: "Invited",
    disabled: "Disabled",
  },

  nav: {
    label: "Main",
    projects: "Projects",
    apps: "Apps",
    services: "Services",
    plugins: "Plugins",
    activity: "Activity",
    users: "Users",
    more: "More",
    server: "Dokku server",
  },

  account: {
    menu: "Account menu",
    signedInAs: "Signed in as",
    theme: "Theme",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
    profile: "Profile",
    settings: "Settings",
    logOut: "Log out",
  },

  login: {
    title: "Sign in to Gokku",
    username: "Username",
    password: "Password",
    submit: "Sign in",
    submitting: "Signing in…",
  },

  setup: {
    title: "Create the first admin",
    intro: "This account can manage every app on this Dokku server. You can invite more people afterwards.",
    username: "Username",
    usernameHint: "Lowercase letters, digits, dashes and underscores.",
    password: "Password",
    passwordHint: (min: number) => `At least ${min} characters.`,
    confirm: "Repeat password",
    submit: "Create admin and sign in",
    submitting: "Creating…",
  },

  invite: {
    title: "Join Gokku",
    intro: "Choose a password to finish setting up your account.",
    username: "Username",
    submit: "Set password and sign in",
    invalidTitle: "This invite link does not work",
    goToSignIn: "Go to sign in",
  },

  validation: {
    required: "Required.",
    passwordMin: (min: number) => `Use at least ${min} characters.`,
    passwordsDiffer: "The passwords do not match.",
    username:
      "Use 3 to 32 lowercase letters, digits, dashes or underscores, starting with a letter or digit.",
    tokenName: "Use 1 to 64 characters.",
  },

  profile: {
    title: "Profile",
    description: "Your account on this Gokku server.",
    accountTitle: "Account",
    username: "Username",
    usernameHint: "Your sessions and API tokens keep working after a rename.",
    usernameSaved: "Username changed.",
    passwordTitle: "Password",
    passwordIntro: "Changing your password signs you out on every other device.",
    currentPassword: "Current password",
    newPassword: "New password",
    confirmPassword: "Repeat new password",
    changePassword: "Change password",
    passwordChanged: "Password changed. Your other sessions are signed out.",
    sessionsTitle: "Sessions",
    sessionsIntro: "Browsers where you are signed in.",
    thisSession: "This session",
    lastActive: (when: string) => `Active ${when}`,
    signOutSession: "Sign out",
    sessionRevoked: "Session signed out.",
  },

  users: {
    title: "Users",
    description: "People who can sign in to Gokku, and API tokens for scripts.",
    invite: "Invite user",
    columns: { user: "User", role: "Role", lastActive: "Last active", status: "Status", actions: "Actions" },
    inviteTitle: "Invite user",
    inviteIntro: "Gokku creates a one-time link. Send it to the person you invite; it works for 7 days.",
    username: "Username",
    role: "Role",
    createInvite: "Create invite link",
    inviteCreatedTitle: "Invite link created",
    inviteCreatedBody: (name: string, expires: string) =>
      `Send this link to ${name}. It works once and expires ${expires}. Gokku does not show it again.`,
    roleMenu: "Role",
    disable: "Disable",
    enable: "Enable",
    newInviteLink: "New invite link",
    delete: "Delete user",
    deleteTitle: (name: string) => `Delete ${name}?`,
    deleteBody: "Their sessions and API tokens stop working. Audit entries keep their name.",
    deleteConfirmLabel: (name: string) => `Type ${name} to confirm`,
    deleted: (name: string) => `${name} was deleted.`,
    updated: (name: string) => `${name} was updated.`,
    pending: (when: string) => `Invite expires ${when}`,
  },

  tokens: {
    title: "API tokens",
    intro: "Tokens act as their owner, limited to the role you choose. Use them with Authorization: Bearer.",
    create: "Create token",
    empty: "No API tokens yet.",
    columns: {
      name: "Name",
      prefix: "Token",
      role: "Role",
      owner: "Owner",
      lastUsed: "Last used",
      created: "Created",
    },
    createTitle: "Create API token",
    name: "Name",
    nameHint: "What uses it, for example ci-deploy.",
    role: "Role",
    submit: "Create token",
    createdTitle: "Token created",
    createdBody: "Copy the token now. Gokku stores only a hash and cannot show it again.",
    revoke: "Revoke",
    revokeTitle: (name: string) => `Revoke ${name}?`,
    revokeBody: "Scripts using this token stop working immediately.",
    revoked: (name: string) => `${name} was revoked.`,
  },

  later: {
    title: (section: string) => `${section} arrive in a later milestone`,
    body: "This part of Gokku is not built yet. Sign-in, your profile and user management work now.",
  },

  confirm: {
    typeToConfirm: (value: string) => `Type ${value} to confirm`,
  },
};
