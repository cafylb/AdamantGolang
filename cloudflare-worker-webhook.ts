// Cloudflare Tunnel Webhook Worker for Telegram Bot
// This worker receives updates from Telegram and forwards them to your bot

export interface Env {
  // Bot server URL - your Go server that handles webhooks
  BOT_SERVER_URL: string;
  // Secret token for verification
  SECRET_TOKEN: string;
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const pathname = url.pathname;

    // Handle CORS preflight
    if (request.method === "OPTIONS") {
      return new Response(null, {
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
          "Access-Control-Allow-Headers": "Content-Type, X-Telegram-Bot-Api-Secret-Token",
        },
      });
    }

    // Telegram webhook endpoint
    if (pathname === "/webhook" || pathname === "/webhook/telegram") {
      return handleTelegramWebhook(request, env);
    }

    // Health check endpoint
    if (pathname === "/api/health") {
      return new Response(JSON.stringify({ ok: true, service: "telegram-webhook" }), {
        headers: { "Content-Type": "application/json" },
      });
    }

    // Set webhook endpoint (for initial setup)
    if (pathname === "/webhook/set" && request.method === "POST") {
      return handleSetWebhook(request, env);
    }

    return new Response("Not Found", { status: 404 });
  },

  // Scheduled handler for periodic tasks (optional)
  async scheduled(event: ScheduledEvent, env: Env, ctx: ExecutionContext): Promise<void> {
    console.log("Scheduled task triggered at", new Date().toISOString());
    // Add your periodic tasks here
  },
};

async function handleTelegramWebhook(request: Request, env: Env): Promise<Response> {
  // Verify secret token
  const secretToken = request.headers.get("X-Telegram-Bot-Api-Secret-Token");
  if (env.SECRET_TOKEN && secretToken !== env.SECRET_TOKEN) {
    console.log("Invalid secret token");
    return new Response("Unauthorized", { status: 401 });
  }

  // Only allow POST requests
  if (request.method !== "POST") {
    return new Response("Method not allowed", { status: 405 });
  }

  try {
    // Parse the incoming update from Telegram
    const update = await request.json();

    // Log the update for debugging
    console.log("Received update:", JSON.stringify(update).substring(0, 500));

    // Forward to your bot's webhook endpoint
    const BOT_SERVER_URL = env.BOT_SERVER_URL;
    if (!BOT_SERVER_URL) {
      console.error("BOT_SERVER_URL not configured");
      return new Response(JSON.stringify({ error: "Bot server not configured" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      });
    }

    const response = await fetch(BOT_SERVER_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Bot-Api-Secret-Token": secretToken || "",
      },
      body: JSON.stringify(update),
    });

    // Get the response from bot server
    const result = await response.text();

    return new Response(result, {
      status: response.status,
      headers: {
        "Content-Type": "application/json",
        "Access-Control-Allow-Origin": "*",
      },
    });
  } catch (error) {
    console.error("Error processing webhook:", error);
    return new Response(JSON.stringify({ error: error instanceof Error ? error.message : "Unknown error" }), {
      status: 500,
      headers: {
        "Content-Type": "application/json",
        "Access-Control-Allow-Origin": "*",
      },
    });
  }
}

async function handleSetWebhook(request: Request, env: Env): Promise<Response> {
  try {
    const body = await request.json();
    const webhookUrl = body.webhook_url;

    if (!webhookUrl) {
      return new Response(JSON.stringify({ error: "webhook_url is required" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      });
    }

    // This endpoint can be used to set the webhook via Telegram API
    // Note: You'll need to add your bot token
    const botToken = body.bot_token;
    if (!botToken) {
      return new Response(JSON.stringify({ error: "bot_token is required" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      });
    }

    const telegramApiUrl = `https://api.telegram.org/bot${botToken}/setWebhook`;
    const response = await fetch(telegramApiUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url: webhookUrl,
        max_connections: 100,
      }),
    });

    const result = await response.json();
    return new Response(JSON.stringify(result), {
      status: response.status,
      headers: { "Content-Type": "application/json" },
    });
  } catch (error) {
    return new Response(JSON.stringify({ error: error instanceof Error ? error.message : "Unknown error" }), {
      status: 500,
      headers: { "Content-Type": "application/json" },
    });
  }
}