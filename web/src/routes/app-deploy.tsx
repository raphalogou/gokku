import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Rocket } from "lucide-react";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import { z } from "zod";
import { Banner } from "~/components/app/banner";
import { Card, CardBody, CardHeader } from "~/components/app/card";
import { CodeBlock } from "~/components/app/code-block";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { jobStatusKind } from "~/components/app/job-view";
import { Skeleton } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button } from "~/components/ui/button";
import { Field } from "~/components/ui/field";
import { Input } from "~/components/ui/input";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { deployApp } from "~/lib/api/gen/client";
import type { Deploy, DeployRequest } from "~/lib/api/gen/model";
import { dateTime, duration, timeAgo } from "~/lib/format";
import { applyServerError, errorMessage } from "~/lib/forms";
import { appDeploysQuery, queryKeys } from "~/lib/queries";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app-deploy";

/** Deploy (DEP-1 to DEP-3): git push instructions, image and repository deploys, history. */
export default function AppDeploy({ params }: Route.ComponentProps) {
  const name = params.name;
  const deploys = useQuery(appDeploysQuery(name));
  const canDeploy = useCan("app.deploy");
  const t = strings.deploy;
  const branch = deploys.data?.deploy_branch || "main";

  return (
    <div className="flex max-w-5xl flex-col gap-6">
      {!canDeploy ? <Banner>{t.readOnly}</Banner> : null}
      <Card>
        <CardHeader title={t.gitTitle} />
        <CardBody>
          <p className="mb-3 text-muted">{t.gitIntro}</p>
          {deploys.data ? (
            <div className="flex flex-col gap-3">
              <CodeBlock value={`git remote add dokku ${deploys.data.git_remote}`} label={t.gitRemote} />
              <CodeBlock value={`git push dokku ${branch}`} label={t.gitPush} />
            </div>
          ) : (
            <Skeleton className="h-24" />
          )}
        </CardBody>
      </Card>
      {canDeploy ? (
        <div className="grid gap-6 lg:grid-cols-2">
          <ImageForm app={name} />
          <RepositoryForm app={name} />
        </div>
      ) : null}
      <Card>
        <CardHeader title={t.historyTitle} />
        {deploys.isPending ? (
          <div className="p-5">
            <Skeleton className="h-32" />
          </div>
        ) : deploys.isError ? (
          <div className="p-5">
            <Banner variant="error">{errorMessage(deploys.error)}</Banner>
          </div>
        ) : deploys.data.deploys.length === 0 ? (
          <p className="px-5 py-6 text-muted">{t.historyEmpty}</p>
        ) : (
          <History app={name} deploys={deploys.data.deploys} />
        )}
      </Card>
    </div>
  );
}

function useDeploy(app: string) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return async (req: DeployRequest) => {
    const { job_id } = await deployApp(app, req);
    notify(strings.deploy.started);
    void qc.invalidateQueries({ queryKey: queryKeys.appDeploys(app) });
    await navigate(`/apps/${app}/deploys/${job_id}`);
  };
}

const imageSchema = z.object({ image: z.string().trim().min(1, strings.validation.required) });

function ImageForm({ app }: { app: string }) {
  const t = strings.deploy;
  const deploy = useDeploy(app);
  const form = useForm<z.infer<typeof imageSchema>>({
    resolver: zodResolver(imageSchema),
    defaultValues: { image: "" },
  });
  const { errors, isSubmitting } = form.formState;
  const onSubmit = form.handleSubmit(async ({ image }) => {
    try {
      await deploy({ source: "image", image });
    } catch (err) {
      applyServerError(err, form.setError, ["image"]);
    }
  });
  return (
    <Card>
      <CardHeader title={t.imageTitle} />
      <CardBody>
        <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
          {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
          <Field label={t.image} hint={t.imageHint} error={errors.image?.message}>
            <Input
              className="font-mono"
              placeholder={`registry.example.com/${app}:tag`}
              autoCapitalize="none"
              spellCheck={false}
              {...form.register("image")}
            />
          </Field>
          <div>
            <Button type="submit" disabled={isSubmitting}>
              <Rocket aria-hidden />
              {isSubmitting ? t.deploying : t.deployImage}
            </Button>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

const repoSchema = z.object({
  url: z.string().trim().min(1, strings.validation.required),
  ref: z.string().trim(),
});

function RepositoryForm({ app }: { app: string }) {
  const t = strings.deploy;
  const deploy = useDeploy(app);
  const form = useForm<z.infer<typeof repoSchema>>({
    resolver: zodResolver(repoSchema),
    defaultValues: { url: "", ref: "" },
  });
  const { errors, isSubmitting } = form.formState;
  const onSubmit = form.handleSubmit(async ({ url, ref }) => {
    try {
      await deploy({ source: "repository", url, ref: ref || undefined });
    } catch (err) {
      applyServerError(err, form.setError, ["url", "ref"]);
    }
  });
  return (
    <Card>
      <CardHeader title={t.repoTitle} />
      <CardBody>
        <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
          {errors.root?.message ? <Banner variant="error">{errors.root.message}</Banner> : null}
          <Field label={t.repoUrl} error={errors.url?.message}>
            <Input
              className="font-mono"
              placeholder="https://github.com/acme/app.git"
              autoCapitalize="none"
              spellCheck={false}
              {...form.register("url")}
            />
          </Field>
          <Field label={t.ref} hint={t.refHint} error={errors.ref?.message}>
            <Input
              className="font-mono"
              placeholder="main"
              autoCapitalize="none"
              spellCheck={false}
              {...form.register("ref")}
            />
          </Field>
          <div>
            <Button type="submit" disabled={isSubmitting}>
              <Rocket aria-hidden />
              {isSubmitting ? t.deploying : t.deployRepo}
            </Button>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

function History({ app, deploys }: { app: string; deploys: Deploy[] }) {
  const t = strings.deploy;
  return (
    <DataTable label={t.historyTitle} className="min-w-[760px]">
      <THead>
        <TH>{t.columns.when}</TH>
        <TH>{t.columns.source}</TH>
        <TH>{t.columns.ref}</TH>
        <TH>{t.columns.result}</TH>
        <TH>{t.columns.duration}</TH>
        <TH>
          <span className="sr-only">{t.columns.output}</span>
        </TH>
      </THead>
      <tbody>
        {deploys.map((d) => (
          <TR key={d.id}>
            <TD>
              <time dateTime={d.started_at} title={dateTime(d.started_at)}>
                {timeAgo(d.started_at)}
              </time>
              {d.creator ? <span className="block text-[13px] text-muted">{d.creator}</span> : null}
            </TD>
            <TD>{t.sources[d.source]}</TD>
            <TD className="max-w-[320px] font-mono text-[13px] break-all">{d.ref}</TD>
            <TD>
              <StatusPill kind={jobStatusKind[d.status]} label={strings.job.status[d.status]} />
            </TD>
            <TD>{d.finished_at && d.source !== "observed" ? duration(d.started_at, d.finished_at) : "—"}</TD>
            <TD>
              {d.job_id ? (
                <Link to={`/apps/${app}/deploys/${d.job_id}`} className="font-semibold hover:underline">
                  {t.viewOutput}
                </Link>
              ) : (
                <span className="text-[13px] text-muted">{t.noOutput}</span>
              )}
            </TD>
          </TR>
        ))}
      </tbody>
    </DataTable>
  );
}
