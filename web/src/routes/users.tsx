import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Ban, CircleCheck, Ellipsis, KeyRound, Link2, Plus, Trash2, UserPlus } from "lucide-react";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { data } from "react-router";
import { z } from "zod";
import { Avatar } from "~/components/app/account-menu";
import { Banner } from "~/components/app/banner";
import { Card, CardHeader } from "~/components/app/card";
import { CodeBlock } from "~/components/app/code-block";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState } from "~/components/app/states";
import { type StatusKind, StatusPill } from "~/components/app/status-pill";
import { TypedConfirmDialog } from "~/components/app/typed-confirm-dialog";
import { Button } from "~/components/ui/button";
import { Dialog, DialogActions } from "~/components/ui/dialog";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import {
  MenuItem,
  MenuLabel,
  MenuPopup,
  MenuRadioGroup,
  MenuRadioItem,
  MenuRoot,
  MenuSeparator,
  MenuTrigger,
} from "~/components/ui/menu";
import { Select } from "~/components/ui/select";
import { notify } from "~/components/ui/toast";
import {
  createToken,
  deleteUser,
  inviteUser,
  resetInvite,
  revokeToken,
  updateUser,
} from "~/lib/api/gen/client";
import type { InviteCreated, Role, Token, TokenCreated, User, UserStatus } from "~/lib/api/gen/model";
import { dateTime, timeAgo } from "~/lib/format";
import { applyServerError, errorMessage } from "~/lib/forms";
import { meQuery, queryKeys, tokensQuery, usersQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { username } from "~/lib/schemas";
import { strings } from "~/lib/strings";

export async function clientLoader() {
  const me = await queryClient.ensureQueryData(meQuery());
  if (me.role !== "admin") throw data(null, { status: 403 });
  await Promise.all([queryClient.ensureQueryData(usersQuery()), queryClient.ensureQueryData(tokensQuery())]);
  return null;
}

const roleOptions: { value: Role; label: string; hint: string }[] = [
  { value: "viewer", label: strings.roles.viewer, hint: strings.roles.viewerHint },
  { value: "operator", label: strings.roles.operator, hint: strings.roles.operatorHint },
  { value: "admin", label: strings.roles.admin, hint: strings.roles.adminHint },
];

const statusKind: Record<UserStatus, StatusKind> = {
  active: "running",
  invited: "deploying",
  disabled: "stopped",
};

export default function Users() {
  const [inviteOpen, setInviteOpen] = useState(false);
  return (
    <>
      <PageHeader
        title={strings.users.title}
        description={strings.users.description}
        actions={
          <Button onClick={() => setInviteOpen(true)}>
            <UserPlus aria-hidden />
            {strings.users.invite}
          </Button>
        }
      />
      <div className="flex flex-col gap-6">
        <UsersCard />
        <TokensCard />
      </div>
      <InviteDialog open={inviteOpen} onOpenChange={setInviteOpen} />
    </>
  );
}

// --- Users --------------------------------------------------------------------------------

function UsersCard() {
  const { data: users } = useSuspenseQuery(usersQuery());
  const { data: me } = useSuspenseQuery(meQuery());
  return (
    <Card>
      <CardHeader title={strings.users.title} />
      <DataTable label={strings.users.title} className="min-w-[640px]">
        <THead>
          <TH>{strings.users.columns.user}</TH>
          <TH>{strings.users.columns.role}</TH>
          <TH>{strings.users.columns.lastActive}</TH>
          <TH>{strings.users.columns.status}</TH>
          <TH className="w-16">
            <span className="sr-only">{strings.users.columns.actions}</span>
          </TH>
        </THead>
        <tbody>
          {users.map((u) => (
            <TR key={u.id}>
              <TD>
                <span className="flex min-w-0 items-center gap-3">
                  <Avatar name={u.username} className="size-8 text-sm" />
                  <span className="truncate font-semibold">{u.username}</span>
                  {u.id === me.id ? (
                    <span className="rounded-full bg-chip px-2 py-0.5 text-[13px] text-muted">
                      {strings.common.you}
                    </span>
                  ) : null}
                </span>
              </TD>
              <TD>{strings.roles[u.role]}</TD>
              <TD className="text-muted">
                {u.status === "invited" && u.invite_expires_at ? (
                  <span title={dateTime(u.invite_expires_at)}>
                    {strings.users.pending(timeAgo(u.invite_expires_at))}
                  </span>
                ) : u.last_active_at ? (
                  <time dateTime={u.last_active_at} title={dateTime(u.last_active_at)}>
                    {timeAgo(u.last_active_at)}
                  </time>
                ) : (
                  strings.common.never
                )}
              </TD>
              <TD>
                <StatusPill kind={statusKind[u.status]} label={strings.status[u.status]} />
              </TD>
              <TD className="py-0 text-right">
                <UserActions user={u} />
              </TD>
            </TR>
          ))}
        </tbody>
      </DataTable>
    </Card>
  );
}

function UserActions({ user }: { user: User }) {
  const qc = useQueryClient();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [invite, setInvite] = useState<InviteCreated | null>(null);
  const refresh = () => qc.invalidateQueries({ queryKey: queryKeys.users() });

  const update = useMutation({
    mutationFn: (body: Parameters<typeof updateUser>[1]) => updateUser(user.id, body),
    onSuccess: () => {
      notify(strings.users.updated(user.username));
      return refresh();
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });
  const remove = useMutation({
    mutationFn: () => deleteUser(user.id),
    onSuccess: () => {
      setDeleteOpen(false);
      notify(strings.users.deleted(user.username));
      return Promise.all([refresh(), qc.invalidateQueries({ queryKey: queryKeys.tokens() })]);
    },
  });
  const newLink = useMutation({
    mutationFn: () => resetInvite(user.id),
    onSuccess: (res) => {
      setInvite(res);
      return refresh();
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  return (
    <>
      <MenuRoot>
        <MenuTrigger
          aria-label={strings.common.moreActions(user.username)}
          className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-muted hover:bg-subtle data-popup-open:bg-subtle"
        >
          <Ellipsis className="size-[18px]" aria-hidden />
        </MenuTrigger>
        <MenuPopup>
          <MenuLabel>{strings.users.roleMenu}</MenuLabel>
          <MenuRadioGroup
            value={user.role}
            onValueChange={(role: Role) => role !== user.role && update.mutate({ role })}
          >
            {roleOptions.map((o) => (
              <MenuRadioItem key={o.value} value={o.value}>
                {o.label}
              </MenuRadioItem>
            ))}
          </MenuRadioGroup>
          <MenuSeparator />
          {user.status === "invited" ? (
            <MenuItem onClick={() => newLink.mutate()}>
              <Link2 aria-hidden />
              {strings.users.newInviteLink}
            </MenuItem>
          ) : user.status === "active" ? (
            <MenuItem onClick={() => update.mutate({ status: "disabled" })}>
              <Ban aria-hidden />
              {strings.users.disable}
            </MenuItem>
          ) : (
            <MenuItem onClick={() => update.mutate({ status: "active" })}>
              <CircleCheck aria-hidden />
              {strings.users.enable}
            </MenuItem>
          )}
          <MenuSeparator />
          <MenuItem danger onClick={() => setDeleteOpen(true)}>
            <Trash2 aria-hidden />
            {strings.users.delete}
          </MenuItem>
        </MenuPopup>
      </MenuRoot>

      <TypedConfirmDialog
        open={deleteOpen}
        onOpenChange={(o) => {
          setDeleteOpen(o);
          if (!o) remove.reset();
        }}
        title={strings.users.deleteTitle(user.username)}
        description={strings.users.deleteBody}
        confirmValue={user.username}
        confirmLabel={strings.users.delete}
        onConfirm={() => remove.mutate()}
        pending={remove.isPending}
        error={remove.error ? errorMessage(remove.error) : undefined}
      />
      <InviteLinkDialog invite={invite} onClose={() => setInvite(null)} />
    </>
  );
}

function InviteDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const qc = useQueryClient();
  const [created, setCreated] = useState<InviteCreated | null>(null);
  const schema = z.object({ username, role: z.enum(["viewer", "operator", "admin"]) });
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: { username: "", role: "viewer" as Role },
  });
  const { errors, isSubmitting } = form.formState;

  const close = () => {
    onOpenChange(false);
    form.reset();
  };
  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const res = await inviteUser(values);
      await qc.invalidateQueries({ queryKey: queryKeys.users() });
      close();
      setCreated(res);
    } catch (err) {
      applyServerError(err, form.setError, ["username", "role"]);
    }
  });

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => (o ? onOpenChange(true) : close())}
        title={strings.users.inviteTitle}
        description={strings.users.inviteIntro}
      >
        <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
          {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
          <Field
            label={strings.users.username}
            hint={strings.setup.usernameHint}
            error={errors.username?.message}
          >
            <Input
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              {...form.register("username")}
            />
          </Field>
          <Controller
            control={form.control}
            name="role"
            render={({ field }) => (
              <Field label={strings.users.role} error={errors.role?.message}>
                <Select value={field.value} onValueChange={field.onChange} options={roleOptions} />
              </Field>
            )}
          />
          <DialogActions>
            <Button variant="secondary" onClick={close}>
              {strings.common.cancel}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {strings.users.createInvite}
            </Button>
          </DialogActions>
        </form>
      </Dialog>
      <InviteLinkDialog invite={created} onClose={() => setCreated(null)} />
    </>
  );
}

/** Shows a one-time invite link (AUTH-7); it cannot be shown again. */
function InviteLinkDialog({ invite, onClose }: { invite: InviteCreated | null; onClose: () => void }) {
  return (
    <Dialog
      open={invite !== null}
      onOpenChange={(o) => !o && onClose()}
      title={strings.users.inviteCreatedTitle}
      description={
        invite ? strings.users.inviteCreatedBody(invite.user.username, timeAgo(invite.expires_at)) : undefined
      }
    >
      {invite ? <CodeBlock value={invite.invite_url} label={strings.users.inviteCreatedTitle} /> : null}
      <DialogActions>
        <Button onClick={onClose}>{strings.common.done}</Button>
      </DialogActions>
    </Dialog>
  );
}

// --- API tokens ---------------------------------------------------------------------------

function TokensCard() {
  const { data: tokens } = useSuspenseQuery(tokensQuery());
  const [createOpen, setCreateOpen] = useState(false);
  const [revoking, setRevoking] = useState<Token | null>(null);
  return (
    <Card>
      <CardHeader
        title={strings.tokens.title}
        description={strings.tokens.intro}
        actions={
          <Button variant="secondary" onClick={() => setCreateOpen(true)}>
            <Plus aria-hidden />
            {strings.tokens.create}
          </Button>
        }
      />
      {tokens.length === 0 ? (
        <EmptyState icon={KeyRound} title={strings.tokens.empty} />
      ) : (
        <DataTable label={strings.tokens.title} className="min-w-[720px]">
          <THead>
            <TH>{strings.tokens.columns.name}</TH>
            <TH>{strings.tokens.columns.prefix}</TH>
            <TH>{strings.tokens.columns.role}</TH>
            <TH>{strings.tokens.columns.owner}</TH>
            <TH>{strings.tokens.columns.lastUsed}</TH>
            <TH>
              <span className="sr-only">{strings.users.columns.actions}</span>
            </TH>
          </THead>
          <tbody>
            {tokens.map((t) => (
              <TR key={t.id}>
                <TD className="font-semibold">{t.name}</TD>
                <TD>
                  <code className="rounded bg-chip px-1.5 py-0.5 font-mono text-[13px]">{t.prefix}…</code>
                </TD>
                <TD>{strings.roles[t.role]}</TD>
                <TD>{t.owner}</TD>
                <TD className="text-muted">
                  {t.last_used_at ? (
                    <time dateTime={t.last_used_at} title={dateTime(t.last_used_at)}>
                      {timeAgo(t.last_used_at)}
                    </time>
                  ) : (
                    strings.common.never
                  )}
                </TD>
                <TD className="text-right">
                  <Button variant="danger" onClick={() => setRevoking(t)}>
                    {strings.tokens.revoke}
                  </Button>
                </TD>
              </TR>
            ))}
          </tbody>
        </DataTable>
      )}
      <CreateTokenDialog open={createOpen} onOpenChange={setCreateOpen} />
      <RevokeTokenDialog token={revoking} onClose={() => setRevoking(null)} />
    </Card>
  );
}

function CreateTokenDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const qc = useQueryClient();
  const [created, setCreated] = useState<TokenCreated | null>(null);
  const schema = z.object({
    name: z.string().trim().min(1, strings.validation.tokenName).max(64, strings.validation.tokenName),
    role: z.enum(["viewer", "operator", "admin"]),
  });
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: { name: "", role: "viewer" as Role },
  });
  const { errors, isSubmitting } = form.formState;

  const close = () => {
    onOpenChange(false);
    form.reset();
  };
  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const res = await createToken(values);
      await qc.invalidateQueries({ queryKey: queryKeys.tokens() });
      close();
      setCreated(res);
    } catch (err) {
      applyServerError(err, form.setError, ["name", "role"]);
    }
  });

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => (o ? onOpenChange(true) : close())}
        title={strings.tokens.createTitle}
      >
        <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
          {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
          <Field label={strings.tokens.name} hint={strings.tokens.nameHint} error={errors.name?.message}>
            <Input autoComplete="off" {...form.register("name")} />
          </Field>
          <Controller
            control={form.control}
            name="role"
            render={({ field }) => (
              <Field label={strings.tokens.role} error={errors.role?.message}>
                <Select value={field.value} onValueChange={field.onChange} options={roleOptions} />
              </Field>
            )}
          />
          <DialogActions>
            <Button variant="secondary" onClick={close}>
              {strings.common.cancel}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {strings.tokens.submit}
            </Button>
          </DialogActions>
        </form>
      </Dialog>
      <Dialog
        open={created !== null}
        onOpenChange={(o) => !o && setCreated(null)}
        title={strings.tokens.createdTitle}
        description={strings.tokens.createdBody}
      >
        {created ? <CodeBlock value={created.secret} label={created.token.name} /> : null}
        <DialogActions>
          <Button onClick={() => setCreated(null)}>{strings.common.done}</Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

function RevokeTokenDialog({ token, onClose }: { token: Token | null; onClose: () => void }) {
  const qc = useQueryClient();
  const revoke = useMutation({
    mutationFn: (t: Token) => revokeToken(t.id),
    onSuccess: (_res, t) => {
      notify(strings.tokens.revoked(t.name));
      onClose();
      return qc.invalidateQueries({ queryKey: queryKeys.tokens() });
    },
  });
  return (
    <Dialog
      open={token !== null}
      onOpenChange={(o) => {
        if (!o) {
          revoke.reset();
          onClose();
        }
      }}
      title={token ? strings.tokens.revokeTitle(token.name) : ""}
      description={strings.tokens.revokeBody}
    >
      {revoke.error ? <Banner variant="error">{errorMessage(revoke.error)}</Banner> : null}
      <DialogActions>
        <Button variant="secondary" onClick={onClose}>
          {strings.common.cancel}
        </Button>
        <Button variant="danger" disabled={revoke.isPending} onClick={() => token && revoke.mutate(token)}>
          {strings.tokens.revoke}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
