export const dynamic = "force-dynamic";

export async function POST(req: Request) {
  const body = await req.json();

  const backendRes = await fetch(
    "http://localhost:8080/action/generate-stream",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  );

  return new Response(backendRes.body, {
    status: backendRes.status,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      "Connection": "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
