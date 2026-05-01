"use client";

import { useState, useEffect, useRef, KeyboardEvent, useCallback } from "react";
import Markdown from "react-markdown";

// ─── Types ────────────────────────────────────────────────────────────────────

interface Project {
  id: string;
  host: string;
  hostPort: string;
  name: string;
}

interface ProjectListItem {
  projectId: string;
  name: string;
  host: string;
  updatedAt?: string;
}

interface ChatMessage {
  ID: string;
  Chat: string;
  Response?: string;
  CreatedAt: string;
  ProjectID: string;
}

interface ProjectFile {
  filename: string;
  code: string;
}

type AppView = "home" | "editor";

// ─── Root component ───────────────────────────────────────────────────────────

export default function VibeCoder() {
  const [view, setView] = useState<AppView>("home");
  const [project, setProject] = useState<Project | null>(null);
  const [chatHistory, setChatHistory] = useState<ChatMessage[]>([]);
  const [prompt, setPrompt] = useState("");
  const [generating, setGenerating] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [generateError, setGenerateError] = useState("");
  const [files, setFiles] = useState<ProjectFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<ProjectFile | null>(null);
  const isGeneratingRef = useRef(false);

  useEffect(() => {
    const saved = localStorage.getItem("vc_project");
    if (!saved) return;
    const p: Project = JSON.parse(saved);
    openProject(p);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const loadProjectData = (p: Project) => {
    Promise.all([
      fetch(`/api/project/${p.id}`).then((r) => r.json()).catch(() => null),
      fetch(`/api/project/${p.id}/files`).then((r) => r.json()).catch(() => null),
      fetch(`/api/project/${p.id}/chat-history`).then((r) => r.json()).catch(() => null),
    ]).then(([projectData, filesData, chatData]) => {
      if (projectData?.data) {
        const updated = { ...p, hostPort: projectData.data.hostPort };
        setProject(updated);
        localStorage.setItem("vc_project", JSON.stringify(updated));
      }
      if (filesData?.files) setFiles(filesData.files);
      if (chatData?.data) setChatHistory(chatData.data);
    });
  };

  const openProject = (p: Project) => {
    setProject(p);
    localStorage.setItem("vc_project", JSON.stringify(p));
    setChatHistory([]);
    setFiles([]);
    setSelectedFile(null);
    setView("editor");
    loadProjectData(p);
  };

  const doGenerate = useCallback(async (activeProject: Project, activePrompt: string) => {
    if (!activePrompt.trim() || isGeneratingRef.current) return;
    isGeneratingRef.current = true;
    setGenerating(true);
    setGenerateError("");

    const optimisticId = `optimistic-${Date.now()}`;
    setChatHistory((prev) => [
      ...prev,
      { ID: optimisticId, Chat: activePrompt.trim(), CreatedAt: new Date().toISOString(), ProjectID: activeProject.id },
    ]);
    setPrompt("");

    try {
      const res = await fetch("/api/action/generate-stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ projectId: activeProject.id, prompt: activePrompt.trim() }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || "Generation failed");
      }

      if (!res.body) throw new Error("No response body");

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split("\n\n");
        buffer = parts.pop() || "";
        for (const part of parts) {
          const lines = part.split("\n");
          let eventType = "message";
          const dataLines: string[] = [];
          for (const line of lines) {
            if (line.startsWith("event: ")) eventType = line.slice(7).trim();
            else if (line.startsWith("data: ")) dataLines.push(line.slice(6));
          }
          const data = dataLines.join("\n");
          if (eventType === "error") throw new Error(data || "Generation error");
          if (eventType === "done" && data) {
            try {
              const parsed = JSON.parse(data) as {
                readyToExecute?: boolean;
                response?: string;
                files?: ProjectFile[];
              };
              if (typeof parsed.readyToExecute === "boolean") {
                if (parsed.response) {
                  setChatHistory((prev) =>
                    prev.map((m) =>
                      m.ID === optimisticId ? { ...m, Response: parsed.response } : m
                    )
                  );
                }
                if (parsed.readyToExecute) {
                  setExecuting(true);
                  setFiles([]);
                  setSelectedFile(null);
                  if (Array.isArray(parsed.files)) {
                    setFiles(parsed.files);
                  }
                }
              } else if (Array.isArray(parsed.files)) {
                setFiles(parsed.files);
                setExecuting(false);
              }
            } catch (_) {
              fetch(`/api/project/${activeProject.id}/files`)
                .then((r) => r.json())
                .then((d) => { if (d.files) setFiles(d.files); })
                .catch(() => {});
            }
          }
        }
      }
    } catch (err: unknown) {
      setChatHistory((prev) => prev.filter((m) => m.ID !== optimisticId));
      setPrompt(activePrompt);
      setGenerateError((err as Error).message);
    } finally {
      isGeneratingRef.current = false;
      setGenerating(false);
      setExecuting(false);
      fetch(`/api/project/${activeProject.id}/chat-history?only_last_chat=true`)
        .then((r) => r.json())
        .then((d) => {
          if (d?.data?.length) {
            const latest: ChatMessage = d.data[0];
            setChatHistory((prev) => {
              const exists = prev.some((m) => m.ID === latest.ID);
              const filtered = prev.filter((m) => m.ID !== optimisticId);
              return exists ? filtered.map((m) => m.ID === latest.ID ? latest : m) : [...filtered, latest];
            });
          }
        })
        .catch(() => {});
    }
  }, []);

  const handleProjectCreated = (p: Project, initialPrompt: string) => {
    setProject(p);
    localStorage.setItem("vc_project", JSON.stringify(p));
    setChatHistory([]);
    setFiles([]);
    setSelectedFile(null);
    setView("editor");
    setTimeout(() => doGenerate(p, initialPrompt), 80);
  };

  const handleOpenProject = (item: ProjectListItem) => {
    openProject({ id: item.projectId, host: item.host, hostPort: "", name: item.name });
  };

  const handleDeleteProject = async (id: string) => {
    await fetch(`/api/project/${id}`, { method: "DELETE" });
    if (project?.id === id) {
      localStorage.removeItem("vc_project");
      setProject(null);
      setChatHistory([]);
      setFiles([]);
      setSelectedFile(null);
      setPrompt("");
      setView("home");
    }
  };

  const handleBackToHome = () => {
    localStorage.removeItem("vc_project");
    setProject(null);
    setChatHistory([]);
    setFiles([]);
    setSelectedFile(null);
    setPrompt("");
    setView("home");
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      if (project) doGenerate(project, prompt);
    }
  };

  if (view === "home") {
    return (
      <HomepageView
        onProjectCreated={handleProjectCreated}
        onOpenProject={handleOpenProject}
        onDeleteProject={handleDeleteProject}
      />
    );
  }

  const previewUrl = project ? `http://${project.host}` : "";

  return (
    <EditorView
      project={project!}
      previewUrl={previewUrl}
      prompt={prompt}
      setPrompt={setPrompt}
      chatHistory={chatHistory}
      generating={generating}
      executing={executing}
      generateError={generateError}
      files={files}
      selectedFile={selectedFile}
      setSelectedFile={setSelectedFile}
      onGenerate={() => project && doGenerate(project, prompt)}
      onKeyDown={handleKeyDown}
      onRefreshPreview={() => setSelectedFile(null)}
      onBackToHome={handleBackToHome}
      onDeleteProject={() => project && handleDeleteProject(project.id)}
    />
  );
}

// ─── Homepage ─────────────────────────────────────────────────────────────────

function HomepageView({
  onProjectCreated,
  onOpenProject,
  onDeleteProject,
}: {
  onProjectCreated: (p: Project, prompt: string) => void;
  onOpenProject: (p: ProjectListItem) => void;
  onDeleteProject: (id: string) => Promise<void>;
}) {
  const [prompt, setPrompt] = useState("");
  const [projectName, setProjectName] = useState("");
  const [projects, setProjects] = useState<ProjectListItem[]>([]);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState<string | null>(null);

  useEffect(() => {
    fetch("/api/project")
      .then((r) => r.json())
      .then((d) => setProjects(d.data || []))
      .catch(() => {});
  }, []);

  const handleSubmit = async () => {
    if (!prompt.trim() || creating) return;
    if (!projectName.trim()) {
      setError("Project name is required.");
      return;
    }
    setCreating(true);
    setError("");
    const name =
      projectName.trim().toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "").slice(0, 40) || "project";
    try {
      const res = await fetch("/api/project", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.err || data.error || "Failed to create project");
      onProjectCreated(
        { id: data.data.projectId, host: data.data.host, hostPort: data.data.hostPort, name },
        prompt.trim()
      );
    } catch (err: unknown) {
      setError((err as Error).message);
      setCreating(false);
    }
  };

  const handleDelete = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    if (!confirm("Delete this project? This cannot be undone.")) return;
    setDeletingId(id);
    await onDeleteProject(id);
    setProjects((prev) => prev.filter((p) => p.projectId !== id));
    setDeletingId(null);
  };

  return (
    <div className="min-h-screen flex flex-col" style={{ background: "#08080d", color: "#f1f5f9" }}>
      {/* Gradient blobs */}
      <div className="fixed inset-0 overflow-hidden pointer-events-none" aria-hidden>
        <div
          style={{
            position: "absolute",
            width: "900px",
            height: "900px",
            borderRadius: "50%",
            background: "radial-gradient(circle, rgba(99,102,241,0.4) 0%, transparent 65%)",
            filter: "blur(90px)",
            top: "-15%",
            left: "-10%",
            animation: "vc-blob1 14s ease-in-out infinite",
          }}
        />
        <div
          style={{
            position: "absolute",
            width: "850px",
            height: "850px",
            borderRadius: "50%",
            background: "radial-gradient(circle, rgba(236,72,153,0.35) 0%, transparent 65%)",
            filter: "blur(90px)",
            bottom: "-20%",
            right: "-5%",
            animation: "vc-blob2 17s ease-in-out infinite",
          }}
        />
        <div
          style={{
            position: "absolute",
            width: "650px",
            height: "650px",
            borderRadius: "50%",
            background: "radial-gradient(circle, rgba(139,92,246,0.3) 0%, transparent 65%)",
            filter: "blur(70px)",
            bottom: "5%",
            left: "25%",
            animation: "vc-blob3 20s ease-in-out infinite",
          }}
        />
        <style>{`
          @keyframes vc-blob1 {
            0%, 100% { transform: translate(0,0) scale(1); }
            33% { transform: translate(50px, -30px) scale(1.08); }
            66% { transform: translate(-25px, 20px) scale(0.94); }
          }
          @keyframes vc-blob2 {
            0%, 100% { transform: translate(0,0) scale(1); }
            33% { transform: translate(-50px, 35px) scale(0.92); }
            66% { transform: translate(35px, -25px) scale(1.07); }
          }
          @keyframes vc-blob3 {
            0%, 100% { transform: translate(0,0) scale(1); }
            50% { transform: translate(25px, -45px) scale(1.06); }
          }
        `}</style>
      </div>

      {/* Header */}
      <header className="relative z-10 h-14 flex items-center px-6">
        <div className="flex items-center gap-2">
          <BoltIcon className="text-indigo-400" size={18} strokeWidth={2.5} />
          <span className="font-semibold text-sm tracking-tight">VibeCoder</span>
        </div>
      </header>

      {/* Hero */}
      <div className="relative z-10 flex flex-col items-center px-4 pt-20 pb-16">
        <h1
          className="text-4xl sm:text-5xl font-bold text-center mb-3"
          style={{ color: "#f8fafc", letterSpacing: "-0.03em", lineHeight: 1.15 }}
        >
          What&apos;s on your mind, AI?
        </h1>
        <p className="text-sm mb-10 text-center" style={{ color: "#4b5563" }}>
          Describe what you want to build and we&apos;ll generate it instantly
        </p>

        {/* Prompt input card */}
        <PromptInputCard
          value={prompt}
          onChange={setPrompt}
          projectName={projectName}
          onProjectNameChange={setProjectName}
          onSubmit={handleSubmit}
          creating={creating}
        />

        {error && (
          <p className="mt-4 text-xs" style={{ color: "#f87171" }}>
            {error}
          </p>
        )}

        {/* Projects grid */}
        {projects.length > 0 && (
          <div className="w-full max-w-5xl mt-16">
            <div className="flex items-center gap-3 mb-5 px-1">
              <h2 className="text-sm font-semibold" style={{ color: "#94a3b8" }}>
                My Projects
              </h2>
              <span
                className="text-[11px] px-2 py-0.5 rounded-full"
                style={{ background: "rgba(99,102,241,0.15)", color: "#818cf8" }}
              >
                {projects.length}
              </span>
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
              {projects.map((p) => (
                <ProjectCard
                  key={p.projectId}
                  project={p}
                  deleting={deletingId === p.projectId}
                  onClick={() => onOpenProject(p)}
                  onDelete={(e) => handleDelete(e, p.projectId)}
                />
              ))}
            </div>
          </div>
        )}

        {projects.length === 0 && (
          <div className="mt-16 text-center" style={{ color: "#1e293b" }}>
            <p className="text-sm">No projects yet — describe something above to get started</p>
          </div>
        )}
      </div>
    </div>
  );
}

function PromptInputCard({
  value,
  onChange,
  projectName,
  onProjectNameChange,
  onSubmit,
  creating,
}: {
  value: string;
  onChange: (v: string) => void;
  projectName: string;
  onProjectNameChange: (v: string) => void;
  onSubmit: () => void;
  creating: boolean;
}) {
  const wrapRef = useRef<HTMLDivElement>(null);

  return (
    <div
      ref={wrapRef}
      className="w-full max-w-2xl rounded-2xl transition-all duration-200"
      style={{
        background: "rgba(255,255,255,0.05)",
        border: "1px solid rgba(255,255,255,0.1)",
        backdropFilter: "blur(16px)",
        boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
      }}
      onFocusCapture={() => {
        if (wrapRef.current) {
          wrapRef.current.style.borderColor = "rgba(99,102,241,0.5)";
          wrapRef.current.style.boxShadow = "0 0 0 3px rgba(99,102,241,0.12), 0 8px 32px rgba(0,0,0,0.4)";
        }
      }}
      onBlurCapture={() => {
        if (wrapRef.current) {
          wrapRef.current.style.borderColor = "rgba(255,255,255,0.1)";
          wrapRef.current.style.boxShadow = "0 8px 32px rgba(0,0,0,0.4)";
        }
      }}
    >
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if ((e.ctrlKey || e.metaKey) && e.key === "Enter") onSubmit();
        }}
        placeholder="Build me a landing page for my SaaS..."
        rows={3}
        autoFocus
        className="w-full px-5 pt-4 pb-2 text-sm outline-none resize-none block"
        style={{ background: "transparent", color: "#f1f5f9" }}
      />

      {/* Project name row */}
      <div
        className="flex items-center gap-2 px-4 py-2 mx-1 mb-1 rounded-lg"
        style={{ borderTop: "1px solid rgba(255,255,255,0.06)" }}
      >
        <FolderIcon />
        <input
          type="text"
          value={projectName}
          onChange={(e) => onProjectNameChange(e.target.value)}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === "Enter") onSubmit();
          }}
          placeholder="Project name"
          className="flex-1 text-xs outline-none bg-transparent"
          style={{ color: "#cbd5e1" }}
          maxLength={40}
        />
      </div>

      <div
        className="flex items-center justify-between px-4 pb-3 pt-1"
        style={{ borderTop: "1px solid rgba(255,255,255,0.06)" }}
      >
        <span className="text-[11px]" style={{ color: "#334155" }}>
          ⌘ Enter to generate
        </span>
        <button
          onClick={onSubmit}
          disabled={creating || !value.trim() || !projectName.trim()}
          className="flex items-center gap-2 text-white text-xs font-medium px-4 py-2 rounded-xl transition-all"
          style={{
            background:
              creating || !value.trim() || !projectName.trim() ? "rgba(99,102,241,0.25)" : "rgba(99,102,241,0.85)",
            cursor: creating || !value.trim() || !projectName.trim() ? "not-allowed" : "pointer",
          }}
          onMouseEnter={(e) => {
            if (!creating && value.trim() && projectName.trim())
              (e.currentTarget as HTMLElement).style.background = "rgba(99,102,241,1)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.background =
              creating || !value.trim() || !projectName.trim() ? "rgba(99,102,241,0.25)" : "rgba(99,102,241,0.85)";
          }}
        >
          {creating ? (
            <>
              <Spinner /> Creating...
            </>
          ) : (
            <>
              <SendIcon /> Generate
            </>
          )}
        </button>
      </div>
    </div>
  );
}

function ProjectCard({
  project,
  deleting,
  onClick,
  onDelete,
}: {
  project: ProjectListItem;
  deleting: boolean;
  onClick: () => void;
  onDelete: (e: React.MouseEvent) => void;
}) {
  const [hovered, setHovered] = useState(false);

  return (
    <div
      className="relative rounded-xl overflow-hidden cursor-pointer group"
      style={{
        border: "1px solid rgba(255,255,255,0.07)",
        background: "rgba(255,255,255,0.03)",
        opacity: deleting ? 0.4 : 1,
        transition: "transform 0.15s, box-shadow 0.15s, opacity 0.2s",
        transform: hovered ? "translateY(-2px)" : "translateY(0)",
        boxShadow: hovered ? "0 8px 24px rgba(0,0,0,0.5)" : "none",
      }}
      onClick={onClick}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {/* Thumbnail */}
      <div
        className="w-full"
        style={{
          aspectRatio: "16/9",
          background: projectGradient(project.projectId),
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <span
          className="text-3xl font-bold uppercase select-none"
          style={{
            color: "rgba(255,255,255,0.2)",
            fontFamily: "monospace",
            letterSpacing: "-0.05em",
          }}
        >
          {project.name.slice(0, 2)}
        </span>
      </div>

      {/* Info */}
      <div className="p-3">
        <p
          className="text-xs font-medium truncate"
          style={{ color: "#e2e8f0" }}
        >
          {project.name}
        </p>
        {project.updatedAt && (
          <p className="text-[11px] mt-0.5 truncate" style={{ color: "#475569" }}>
            {formatRelativeTime(project.updatedAt)}
          </p>
        )}
      </div>

      {/* Delete button */}
      {hovered && !deleting && (
        <button
          onClick={onDelete}
          className="absolute top-2 right-2 p-1.5 rounded-lg"
          style={{
            background: "rgba(0,0,0,0.65)",
            color: "#f87171",
            backdropFilter: "blur(4px)",
            border: "1px solid rgba(239,68,68,0.2)",
          }}
          title="Delete project"
        >
          <TrashIcon size={12} />
        </button>
      )}
    </div>
  );
}

// ─── Editor view ──────────────────────────────────────────────────────────────

function EditorView({
  project,
  previewUrl,
  prompt,
  setPrompt,
  chatHistory,
  generating,
  executing,
  generateError,
  files,
  selectedFile,
  setSelectedFile,
  onGenerate,
  onKeyDown,
  onRefreshPreview,
  onBackToHome,
  onDeleteProject,
}: {
  project: Project;
  previewUrl: string;
  prompt: string;
  setPrompt: (v: string) => void;
  chatHistory: ChatMessage[];
  generating: boolean;
  executing: boolean;
  generateError: string;
  files: ProjectFile[];
  selectedFile: ProjectFile | null;
  setSelectedFile: (f: ProjectFile | null) => void;
  onGenerate: () => void;
  onKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  onRefreshPreview: () => void;
  onBackToHome: () => void;
  onDeleteProject: () => void;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [isDragging, setIsDragging] = useState(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const wasGeneratingRef = useRef(false);
  const dragStartXRef = useRef(0);
  const dragStartWidthRef = useRef(0);

  // Reload the iframe only when file execution completes
  useEffect(() => {
    const wasGenerating = wasGeneratingRef.current;
    wasGeneratingRef.current = executing;
    if (wasGenerating && !executing && iframeRef.current && previewUrl) {
      iframeRef.current.src = previewUrl;
    }
  }, [executing, previewUrl]);

  const handleRefresh = () => {
    onRefreshPreview(); // clears selected file
    if (!selectedFile && iframeRef.current && previewUrl) {
      iframeRef.current.src = previewUrl;
    }
  };

  const handleDeleteClick = () => {
    if (confirmDelete) {
      onDeleteProject();
    } else {
      setConfirmDelete(true);
      setTimeout(() => setConfirmDelete(false), 3000);
    }
  };

  const handleDragMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    dragStartXRef.current = e.clientX;
    dragStartWidthRef.current = sidebarWidth;
    setIsDragging(true);
  };

  return (
    <div
      className="h-screen flex flex-col overflow-hidden"
      style={{ background: "#0d0d0f", color: "#f1f5f9" }}
    >
      {/* Header */}
      <header
        className="h-14 flex items-center px-4 flex-shrink-0 gap-3"
        style={{ borderBottom: "1px solid rgba(255,255,255,0.06)" }}
      >
        <button
          onClick={onBackToHome}
          className="p-1.5 rounded-md flex items-center gap-1 text-xs transition-colors"
          style={{ color: "#475569" }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLElement).style.color = "#cbd5e1";
            (e.currentTarget as HTMLElement).style.background = "rgba(255,255,255,0.06)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.color = "#475569";
            (e.currentTarget as HTMLElement).style.background = "transparent";
          }}
          title="Back to home"
        >
          <BackIcon />
        </button>

        <div className="w-px h-4" style={{ background: "rgba(255,255,255,0.08)" }} />

        <div className="flex items-center gap-2">
          <BoltIcon className="text-indigo-400" size={16} strokeWidth={2.5} />
          <span className="font-semibold text-sm tracking-tight">VibeCoder</span>
        </div>

        <div
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded-md"
          style={{ color: "#94a3b8", background: "rgba(255,255,255,0.04)" }}
        >
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 flex-shrink-0" />
          <span className="truncate max-w-40">{project.name}</span>
        </div>

        <div className="ml-auto">
          <button
            onClick={handleDeleteClick}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-md transition-all"
            style={{
              color: confirmDelete ? "#fca5a5" : "#64748b",
              background: confirmDelete ? "rgba(239,68,68,0.12)" : "transparent",
              border: confirmDelete ? "1px solid rgba(239,68,68,0.3)" : "1px solid transparent",
            }}
            onMouseEnter={(e) => {
              if (!confirmDelete) {
                (e.currentTarget as HTMLElement).style.color = "#f87171";
                (e.currentTarget as HTMLElement).style.background = "rgba(239,68,68,0.08)";
              }
            }}
            onMouseLeave={(e) => {
              if (!confirmDelete) {
                (e.currentTarget as HTMLElement).style.color = "#64748b";
                (e.currentTarget as HTMLElement).style.background = "transparent";
              }
            }}
          >
            <TrashIcon size={12} />
            {confirmDelete ? "Confirm delete?" : "Delete"}
          </button>
        </div>
      </header>

      {/* Body */}
      <div className="flex flex-1 overflow-hidden">
        <aside
          className="flex-shrink-0 flex flex-col overflow-hidden"
          style={{ width: sidebarWidth }}
        >
          <PromptPanel
            prompt={prompt}
            setPrompt={setPrompt}
            onGenerate={onGenerate}
            generating={generating}
            executing={executing}
            error={generateError}
            chatHistory={chatHistory}
            onSelectPrompt={setPrompt}
            onKeyDown={onKeyDown}
            files={files}
            onSelectFile={setSelectedFile}
          />
        </aside>

        {/* Drag handle */}
        <div
          onMouseDown={handleDragMouseDown}
          style={{
            width: 4,
            flexShrink: 0,
            cursor: "col-resize",
            background: "rgba(255,255,255,0.06)",
            transition: "background 0.15s",
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLElement).style.background = "rgba(99,102,241,0.5)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.background = "rgba(255,255,255,0.06)";
          }}
        />

        <div className="flex-1 flex flex-col overflow-hidden">
          <UrlBar
            url={previewUrl}
            showPreviewButton={!!selectedFile}
            onRefresh={handleRefresh}
          />
          <div className="flex-1 relative">
            {selectedFile ? (
              <CodeViewer file={selectedFile} onClose={() => setSelectedFile(null)} />
            ) : (
              <>
                {executing && <GeneratingOverlay />}
                <iframe
                  ref={iframeRef}
                  src={previewUrl}
                  className="w-full h-full"
                  style={{ border: "none", background: "#fff" }}
                  title="Preview"
                />
              </>
            )}
          </div>
        </div>
      </div>

      {/* Full-screen overlay while dragging — sits above the iframe so mouse events aren't swallowed */}
      {isDragging && (
        <div
          style={{ position: "fixed", inset: 0, zIndex: 9999, cursor: "col-resize" }}
          onMouseMove={(e) => {
            const delta = e.clientX - dragStartXRef.current;
            setSidebarWidth(Math.min(600, Math.max(200, dragStartWidthRef.current + delta)));
          }}
          onMouseUp={() => setIsDragging(false)}
        />
      )}
    </div>
  );
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function PromptPanel({
  prompt,
  setPrompt,
  onGenerate,
  generating,
  executing,
  error,
  chatHistory,
  onSelectPrompt,
  onKeyDown,
  files,
  onSelectFile,
}: {
  prompt: string;
  setPrompt: (v: string) => void;
  onGenerate: () => void;
  generating: boolean;
  executing: boolean;
  error: string;
  chatHistory: ChatMessage[];
  onSelectPrompt: (t: string) => void;
  onKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  files: ProjectFile[];
  onSelectFile: (f: ProjectFile) => void;
}) {
  const [activeTab, setActiveTab] = useState<"chat" | "files">("chat");
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaWrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (activeTab === "chat") {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [chatHistory, generating, activeTab]);

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <div
        className="flex h-10 items-center px-2 gap-0.5 flex-shrink-0"
        style={{ borderBottom: "1px solid rgba(255,255,255,0.06)" }}
      >
        <SidebarTabBtn
          active={activeTab === "chat"}
          onClick={() => setActiveTab("chat")}
          icon={<ChatTabIcon />}
          label="Chat"
        />
        <SidebarTabBtn
          active={activeTab === "files"}
          onClick={() => setActiveTab("files")}
          icon={<FilesTabIcon />}
          label="Files"
          badge={files.length > 0 ? files.length : undefined}
        />
      </div>

      {activeTab === "chat" && (
        <div className="flex flex-col flex-1 overflow-hidden">
          <div className="flex-1 overflow-y-auto px-3 py-4 flex flex-col gap-3 min-h-0">
            {chatHistory.length === 0 && !generating && (
              <div className="flex flex-col items-center justify-center flex-1 gap-1.5 py-10">
                <ChatTabIcon size={24} muted />
                <p className="text-xs text-center" style={{ color: "#334155" }}>
                  Describe what you want to build
                </p>
              </div>
            )}
            {chatHistory.map((msg) => (
              <div key={msg.ID} className="flex flex-col gap-2">
                <div className="flex justify-end">
                  <div
                    className="max-w-[88%] rounded-2xl rounded-tr-sm px-3 py-2 cursor-pointer vc-markdown vc-markdown-user"
                    style={{
                      background: "rgba(79,70,229,0.2)",
                      border: "1px solid rgba(99,102,241,0.25)",
                    }}
                    onClick={() => onSelectPrompt(msg.Chat)}
                  >
                    <Markdown>{msg.Chat}</Markdown>
                    <p className="text-[10px] mt-1 text-right" style={{ color: "#6366f1" }}>
                      {formatTime(msg.CreatedAt)}
                    </p>
                  </div>
                </div>
                {msg.Response && (
                  <div className="flex justify-start">
                    <div
                      className="max-w-[88%] rounded-2xl rounded-tl-sm px-3 py-2 vc-markdown"
                      style={{
                        background: "rgba(255,255,255,0.05)",
                        border: "1px solid rgba(255,255,255,0.08)",
                      }}
                    >
                      <Markdown>{msg.Response}</Markdown>
                    </div>
                  </div>
                )}
              </div>
            ))}
            {generating && !executing && (
              <div className="flex justify-start">
                <div
                  className="rounded-2xl rounded-tl-sm px-3 py-2.5"
                  style={{
                    background: "rgba(255,255,255,0.05)",
                    border: "1px solid rgba(255,255,255,0.08)",
                  }}
                >
                  <TypingDots />
                </div>
              </div>
            )}
            {executing && (
              <div className="flex justify-start">
                <div
                  className="flex items-center gap-2 rounded-2xl rounded-tl-sm px-3 py-2"
                  style={{
                    background: "rgba(99,102,241,0.08)",
                    border: "1px solid rgba(99,102,241,0.2)",
                  }}
                >
                  <Spinner />
                  <span className="text-xs" style={{ color: "#818cf8" }}>
                    Building your page…
                  </span>
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          <div
            className="p-3 flex-shrink-0"
            style={{ borderTop: "1px solid rgba(255,255,255,0.06)" }}
          >
            {error && (
              <p className="text-xs mb-2 px-1" style={{ color: "#f87171" }}>
                {error}
              </p>
            )}
            <div
              ref={textareaWrapRef}
              className="rounded-xl transition-all"
              style={{
                background: "rgba(255,255,255,0.04)",
                border: "1px solid rgba(255,255,255,0.08)",
              }}
              onFocusCapture={() => {
                if (textareaWrapRef.current) {
                  textareaWrapRef.current.style.borderColor = "rgba(99,102,241,0.5)";
                  textareaWrapRef.current.style.boxShadow = "0 0 0 3px rgba(99,102,241,0.1)";
                }
              }}
              onBlurCapture={() => {
                if (textareaWrapRef.current) {
                  textareaWrapRef.current.style.borderColor = "rgba(255,255,255,0.08)";
                  textareaWrapRef.current.style.boxShadow = "none";
                }
              }}
            >
              <textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={onKeyDown}
                placeholder="Describe what you want to build..."
                rows={3}
                className="w-full px-3 pt-2.5 pb-1 text-sm outline-none resize-none block"
                style={{ background: "transparent", color: "#f1f5f9" }}
              />
              <div className="flex items-center justify-between px-2 pb-2">
                <span className="text-[10px]" style={{ color: "#334155" }}>
                  ⌘ Enter
                </span>
                <button
                  onClick={onGenerate}
                  disabled={generating || !prompt.trim()}
                  className="flex items-center gap-1.5 text-white text-xs font-medium px-3 py-1.5 rounded-lg transition-colors"
                  style={{
                    background: "#4f46e5",
                    opacity: generating || !prompt.trim() ? 0.45 : 1,
                    cursor: generating || !prompt.trim() ? "not-allowed" : "pointer",
                  }}
                  onMouseEnter={(e) => {
                    if (!generating && prompt.trim())
                      (e.currentTarget as HTMLElement).style.background = "#4338ca";
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLElement).style.background = "#4f46e5";
                  }}
                >
                  {generating ? (
                    <>
                      <Spinner /> Generating
                    </>
                  ) : (
                    <>
                      <SendIcon /> Send
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {activeTab === "files" && (
        <div className="flex flex-col flex-1 overflow-hidden">
          {files.length === 0 ? (
            <div className="flex flex-col items-center justify-center flex-1 gap-1.5 py-10">
              <FilesTabIcon size={24} muted />
              <p className="text-xs" style={{ color: "#334155" }}>
                No files generated yet
              </p>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto p-2">
              {files.map((f) => (
                <button
                  key={f.filename}
                  onClick={() => onSelectFile(f)}
                  className="w-full text-left p-2.5 rounded-lg mb-0.5 transition-colors flex items-center gap-2.5"
                  onMouseEnter={(e) =>
                    (e.currentTarget.style.background = "rgba(255,255,255,0.05)")
                  }
                  onMouseLeave={(e) =>
                    (e.currentTarget.style.background = "transparent")
                  }
                >
                  <FileTypeBadge filename={f.filename} />
                  <span className="text-xs font-mono truncate" style={{ color: "#cbd5e1" }}>
                    {f.filename}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function SidebarTabBtn({
  active,
  onClick,
  icon,
  label,
  badge,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  badge?: number;
}) {
  return (
    <button
      onClick={onClick}
      className="relative flex items-center gap-1.5 px-3 h-full text-xs font-medium transition-colors rounded-md"
      style={{ color: active ? "#e2e8f0" : "#475569" }}
      onMouseEnter={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.color = "#94a3b8";
      }}
      onMouseLeave={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.color = "#475569";
      }}
    >
      {icon}
      {label}
      {badge !== undefined && (
        <span
          className="text-[10px] font-bold px-1 py-0.5 rounded"
          style={{
            background: "rgba(99,102,241,0.2)",
            color: "#818cf8",
            minWidth: 16,
            textAlign: "center",
          }}
        >
          {badge}
        </span>
      )}
      {active && (
        <span
          className="absolute bottom-0 left-2 right-2 h-0.5 rounded-t-full"
          style={{ background: "#6366f1" }}
        />
      )}
    </button>
  );
}

function TypingDots() {
  return (
    <div className="flex items-center gap-1">
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className="w-1.5 h-1.5 rounded-full"
          style={{
            background: "#475569",
            animation: "vc-typing 1.2s ease-in-out infinite",
            animationDelay: `${i * 0.2}s`,
          }}
        />
      ))}
      <style>{`
        @keyframes vc-typing {
          0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
          30% { transform: translateY(-4px); opacity: 1; }
        }
        .vc-markdown { font-size: 0.75rem; line-height: 1.6; color: #cbd5e1; }
        .vc-markdown p { margin: 0 0 0.5em; }
        .vc-markdown p:last-child { margin-bottom: 0; }
        .vc-markdown strong { color: #f1f5f9; font-weight: 600; }
        .vc-markdown em { color: #94a3b8; font-style: italic; }
        .vc-markdown code { font-family: monospace; font-size: 0.7rem; background: rgba(255,255,255,0.08); color: #a5b4fc; padding: 0.1em 0.35em; border-radius: 4px; }
        .vc-markdown pre { background: rgba(0,0,0,0.3); border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 0.75em 1em; overflow-x: auto; margin: 0.5em 0; }
        .vc-markdown pre code { background: none; padding: 0; color: #e2e8f0; font-size: 0.7rem; }
        .vc-markdown ul, .vc-markdown ol { padding-left: 1.25em; margin: 0.4em 0; }
        .vc-markdown li { margin: 0.2em 0; }
        .vc-markdown h1, .vc-markdown h2, .vc-markdown h3 { color: #f1f5f9; font-weight: 600; margin: 0.6em 0 0.3em; }
        .vc-markdown h1 { font-size: 0.9rem; }
        .vc-markdown h2 { font-size: 0.85rem; }
        .vc-markdown h3 { font-size: 0.8rem; }
        .vc-markdown blockquote { border-left: 2px solid rgba(99,102,241,0.5); margin: 0.4em 0; padding-left: 0.75em; color: #94a3b8; }
        .vc-markdown a { color: #818cf8; text-decoration: underline; }
        .vc-markdown hr { border: none; border-top: 1px solid rgba(255,255,255,0.08); margin: 0.5em 0; }
        .vc-markdown-user { color: #e2e8f0; }
        .vc-markdown-user strong { color: #ffffff; }
        .vc-markdown-user code { background: rgba(99,102,241,0.2); color: #c7d2fe; }
        .vc-markdown-user pre { background: rgba(0,0,0,0.25); border-color: rgba(99,102,241,0.2); }
        .vc-markdown-user blockquote { border-left-color: rgba(199,210,254,0.4); color: #a5b4fc; }
      `}</style>
    </div>
  );
}

function FileTypeBadge({ filename }: { filename: string }) {
  const ext = filename.split(".").pop()?.toLowerCase() || "";
  const styles: Record<string, { bg: string; color: string }> = {
    html: { bg: "rgba(234,88,12,0.15)", color: "#fb923c" },
    css: { bg: "rgba(59,130,246,0.15)", color: "#60a5fa" },
    js: { bg: "rgba(234,179,8,0.15)", color: "#facc15" },
    ts: { bg: "rgba(59,130,246,0.2)", color: "#93c5fd" },
    json: { bg: "rgba(168,85,247,0.15)", color: "#c084fc" },
  };
  const s = styles[ext] || { bg: "rgba(100,116,139,0.15)", color: "#94a3b8" };
  return (
    <span
      className="text-[10px] font-bold px-1.5 py-0.5 rounded flex-shrink-0"
      style={{
        background: s.bg,
        color: s.color,
        fontFamily: "monospace",
        letterSpacing: "0.04em",
      }}
    >
      {ext.toUpperCase() || "FILE"}
    </span>
  );
}

function CodeViewer({ file, onClose }: { file: ProjectFile; onClose: () => void }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(file.code);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="absolute inset-0 z-20 flex flex-col" style={{ background: "#0d0d0f" }}>
      <div
        className="h-12 flex items-center px-4 gap-3 flex-shrink-0"
        style={{ borderBottom: "1px solid rgba(255,255,255,0.06)" }}
      >
        <FileTypeBadge filename={file.filename} />
        <span className="text-sm font-mono" style={{ color: "#cbd5e1" }}>
          {file.filename}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={copy}
            className="text-xs px-2.5 py-1 rounded-md transition-colors"
            style={{
              color: copied ? "#4ade80" : "#64748b",
              background: "rgba(255,255,255,0.04)",
              border: "1px solid rgba(255,255,255,0.08)",
            }}
          >
            {copied ? "Copied!" : "Copy"}
          </button>
          <IconButton onClick={onClose} title="Back to preview">
            <CloseIcon />
          </IconButton>
        </div>
      </div>
      <div className="flex-1 overflow-auto">
        <pre
          className="text-xs leading-relaxed p-6 m-0"
          style={{
            color: "#e2e8f0",
            fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace",
            tabSize: 2,
            whiteSpace: "pre",
          }}
        >
          {file.code}
        </pre>
      </div>
    </div>
  );
}

function UrlBar({
  url,
  showPreviewButton,
  onRefresh,
}: {
  url: string;
  showPreviewButton: boolean;
  onRefresh: () => void;
}) {
  return (
    <div
      className="h-12 flex items-center px-4 gap-2 flex-shrink-0"
      style={{ borderBottom: "1px solid rgba(255,255,255,0.06)" }}
    >
      <IconButton onClick={onRefresh} title={showPreviewButton ? "Back to preview" : "Refresh"}>
        {showPreviewButton ? <PreviewIcon /> : <RefreshIcon />}
      </IconButton>
      <div
        className="flex-1 rounded-md px-3 py-1.5 text-xs font-mono truncate"
        style={{
          background: "rgba(255,255,255,0.04)",
          border: "1px solid rgba(255,255,255,0.06)",
          color: "#64748b",
        }}
      >
        {url || "about:blank"}
      </div>
      {url && (
        <IconButton onClick={() => window.open(url, "_blank")} title="Open in new tab">
          <ExternalIcon />
        </IconButton>
      )}
    </div>
  );
}

function IconButton({
  onClick,
  title,
  children,
}: {
  onClick: () => void;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="p-1.5 rounded-md transition-colors"
      style={{ color: "#64748b" }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.color = "#cbd5e1";
        (e.currentTarget as HTMLElement).style.background = "rgba(255,255,255,0.06)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.color = "#64748b";
        (e.currentTarget as HTMLElement).style.background = "transparent";
      }}
    >
      {children}
    </button>
  );
}

function GeneratingOverlay() {
  return (
    <div
      className="absolute inset-0 z-10 flex flex-col items-center justify-center"
      style={{ background: "rgba(13,13,15,0.85)", backdropFilter: "blur(4px)" }}
    >
      <Spinner size="lg" />
      <p className="text-sm mt-3" style={{ color: "#94a3b8" }}>
        Generating your page...
      </p>
    </div>
  );
}

// ─── Icons ────────────────────────────────────────────────────────────────────

function BoltIcon({
  size = 16,
  strokeWidth = 2,
  className = "",
}: {
  size?: number;
  strokeWidth?: number;
  className?: string;
}) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
    >
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  );
}

function Spinner({ size = "sm" }: { size?: "sm" | "lg" }) {
  const sz = size === "lg" ? 32 : 14;
  return (
    <svg
      width={sz}
      height={sz}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className="animate-spin"
      style={{ color: "#818cf8" }}
    >
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" opacity="0.2" />
      <path fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" opacity="0.8" />
    </svg>
  );
}

function TrashIcon({ size = 14 }: { size?: number }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
      <path d="M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2" />
    </svg>
  );
}

function BackIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="15 18 9 12 15 6" />
    </svg>
  );
}

function RefreshIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
    </svg>
  );
}

function PreviewIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
      <line x1="8" y1="21" x2="16" y2="21" />
      <line x1="12" y1="17" x2="12" y2="21" />
    </svg>
  );
}

function ExternalIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6" />
      <polyline points="15 3 21 3 21 9" />
      <line x1="10" y1="14" x2="21" y2="3" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function ChatTabIcon({ size = 14, muted = false }: { size?: number; muted?: boolean }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ opacity: muted ? 0.3 : 1, flexShrink: 0 }}
    >
      <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
    </svg>
  );
}

function FilesTabIcon({ size = 14, muted = false }: { size?: number; muted?: boolean }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ opacity: muted ? 0.3 : 1, flexShrink: 0 }}
    >
      <path d="M13 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V9z" />
      <polyline points="13 2 13 9 20 9" />
    </svg>
  );
}

function FolderIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ color: "#475569", flexShrink: 0 }}
    >
      <path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z" />
    </svg>
  );
}

function SendIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="22" y1="2" x2="11" y2="13" />
      <polygon points="22 2 15 22 11 13 2 9 22 2" />
    </svg>
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function projectGradient(id: string): string {
  const hash = id.split("").reduce((acc, ch) => acc + ch.charCodeAt(0), 0);
  const hue1 = hash % 360;
  const hue2 = (hash * 37 + 120) % 360;
  return `linear-gradient(135deg, hsl(${hue1},55%,20%), hsl(${hue2},50%,13%))`;
}

function formatTime(ts: string | number): string {
  const d = new Date(ts);
  return (
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) +
    " · " +
    d.toLocaleDateString([], { month: "short", day: "numeric" })
  );
}

function formatRelativeTime(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(ts).toLocaleDateString([], { month: "short", day: "numeric" });
}
