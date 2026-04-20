import React, { useCallback, useEffect, useState } from 'react';

type SessionResponse = {
  session_id: string;
  authenticated: boolean;
};

type ReportResponse = {
  url: string;
  cached: boolean;
  object_key: string;
};

const authBaseUrl = process.env.REACT_APP_AUTH_URL;
const apiBaseUrl = process.env.REACT_APP_API_URL;

const getSessionIdFromHeaders = (response: Response): string | null =>
  response.headers.get('X-Session-Id') || response.headers.get('X-Session-ID');

const ReportPage: React.FC = () => {
  const [initializing, setInitializing] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [session, setSession] = useState<SessionResponse | null>(null);
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');

  const isAuthenticated = Boolean(session?.authenticated);

  const syncSession = useCallback(async () => {
    try {
      const response = await fetch(`${authBaseUrl}/auth/session`, {
        credentials: 'include'
      });

      if (!response.ok) {
        setSession(null);
        return;
      }

      const payload = (await response.json()) as SessionResponse;
      setSession(payload.authenticated ? payload : null);
    } catch (err) {
      setSession(null);
      setError(err instanceof Error ? err.message : 'Failed to load session');
    }
  }, []);

  useEffect(() => {
    let active = true;

    const loadSession = async () => {
      setInitializing(true);
      setError(null);

      try {
        const response = await fetch(`${authBaseUrl}/auth/session`, {
          credentials: 'include'
        });

        if (!active) {
          return;
        }

        if (!response.ok) {
          setSession(null);
          return;
        }

        const payload = (await response.json()) as SessionResponse;
        setSession(payload.authenticated ? payload : null);
      } catch (err) {
        if (active) {
          setError(err instanceof Error ? err.message : 'Failed to load session');
          setSession(null);
        }
      } finally {
        if (active) {
          setInitializing(false);
        }
      }
    };

    void loadSession();

    return () => {
      active = false;
    };
  }, []);

  const login = () => {
    window.location.assign(`${authBaseUrl}/auth/login`);
  };

  const downloadReport = async () => {
    if (!isAuthenticated) {
      setError('Not authenticated');
      return;
    }

    if (!dateFrom || !dateTo) {
      setError('Please select both dates');
      return;
    }

    try {
      setLoading(true);
      setError(null);
      const query = new URLSearchParams({
        date_from: dateFrom,
        date_to: dateTo
      });

      const response = await fetch(`${apiBaseUrl}/reports?${query.toString()}`, {
        credentials: 'include'
      });

      const rotatedSessionId = getSessionIdFromHeaders(response);
      if (rotatedSessionId && session) {
        setSession({ ...session, session_id: rotatedSessionId });
      }

      if (response.status === 401) {
        await syncSession();
        setError('Session expired. Please login again.');
        return;
      }

      if (!response.ok) {
        throw new Error(`Report request failed with status ${response.status}`);
      }

      const payload = (await response.json()) as ReportResponse;
      window.location.assign(payload.url);

      await syncSession();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
    } finally {
      setLoading(false);
    }
  };

  if (initializing) {
    return <div>Loading...</div>;
  }

  if (!isAuthenticated) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen bg-gray-100">
        <button
          onClick={login}
          className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
        >
          Login
        </button>
        {error && (
          <div className="mt-4 p-4 bg-red-100 text-red-700 rounded">
            {error}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-gray-100">
      <div className="p-8 bg-white rounded-lg shadow-md">
        <h1 className="text-2xl font-bold mb-6">Usage Reports</h1>

        <div className="grid grid-cols-1 gap-4 mb-6 sm:grid-cols-2">
          <label className="flex flex-col text-sm text-gray-700">
            <span className="mb-1">Date from</span>
            <input
              type="date"
              value={dateFrom}
              onChange={(event) => setDateFrom(event.target.value)}
              className="rounded border border-gray-300 px-3 py-2"
            />
          </label>

          <label className="flex flex-col text-sm text-gray-700">
            <span className="mb-1">Date to</span>
            <input
              type="date"
              value={dateTo}
              onChange={(event) => setDateTo(event.target.value)}
              className="rounded border border-gray-300 px-3 py-2"
            />
          </label>
        </div>

        <button
          onClick={downloadReport}
          disabled={loading || !dateFrom || !dateTo}
          className={`px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 ${
            loading || !dateFrom || !dateTo ? 'opacity-50 cursor-not-allowed' : ''
          }`}
        >
          {loading ? 'Generating Report...' : 'Download Report'}
        </button>

        {error && (
          <div className="mt-4 p-4 bg-red-100 text-red-700 rounded">
            {error}
          </div>
        )}
      </div>
    </div>
  );
};

export default ReportPage;
