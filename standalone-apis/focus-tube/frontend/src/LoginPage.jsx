import { useState } from "react";
import { requestCode, verifyCode, setToken, setEmail as saveEmail } from "./api";

export default function LoginPage({ onLogin }) {
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [step, setStep] = useState(1); // 1=email, 2=code
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleRequestCode(e) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await requestCode(email);
      if (res.error) {
        setError(res.error);
        return;
      }
      setCode(res.code || "");
      setStep(2);
      // Auto-verify since we have the code
      if (res.code) {
        await doVerify(email, res.code);
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  async function doVerify(em, cd) {
    try {
      const res = await verifyCode(em, cd);
      setToken(res.token);
      saveEmail(res.email);
      onLogin();
    } catch (err) {
      setError(err.message);
    }
  }

  async function handleVerifyCode(e) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await doVerify(email, code);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex items-center justify-center h-screen bg-neutral-950 text-neutral-100">
      <div className="w-80 space-y-6">
        <h1 className="text-2xl font-bold text-red-500 text-center">FocusTube</h1>

        {step === 1 && (
          <form onSubmit={handleRequestCode} className="space-y-4">
            <input
              type="email"
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="w-full px-3 py-2 bg-neutral-800 border border-neutral-700 rounded text-sm focus:outline-none focus:border-red-500"
            />
            <button
              type="submit"
              disabled={loading}
              className="w-full py-2 bg-red-600 rounded text-sm font-medium hover:bg-red-700 disabled:opacity-50"
            >
              {loading ? "..." : "Sign in"}
            </button>
          </form>
        )}

        {step === 2 && (
          <form onSubmit={handleVerifyCode} className="space-y-4">
            <p className="text-sm text-neutral-400 text-center">Enter code for {email}</p>
            <input
              type="text"
              placeholder="6-digit code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
              className="w-full px-3 py-2 bg-neutral-800 border border-neutral-700 rounded text-sm text-center tracking-widest focus:outline-none focus:border-red-500"
            />
            <button
              type="submit"
              disabled={loading}
              className="w-full py-2 bg-red-600 rounded text-sm font-medium hover:bg-red-700 disabled:opacity-50"
            >
              {loading ? "..." : "Verify"}
            </button>
            <button
              type="button"
              onClick={() => setStep(1)}
              className="w-full text-xs text-neutral-500 hover:text-neutral-300"
            >
              Back
            </button>
          </form>
        )}

        {error && <p className="text-red-400 text-xs text-center">{error}</p>}
      </div>
    </div>
  );
}
