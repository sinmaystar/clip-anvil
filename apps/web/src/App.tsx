import { lazy, Suspense, useEffect } from "react";
import { Navigate, RouterProvider, createBrowserRouter } from "react-router";
import { GuestRoute } from "./components/GuestRoute";
import { Layout } from "./components/Layout";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { LoginPage } from "./pages/LoginPage";
import { RegisterPage } from "./pages/RegisterPage";
import { WorkspaceListPage } from "./pages/WorkspaceListPage";
import { useAppearanceStore } from "./stores/appearance";

const WorkspaceDetailPage = lazy(() =>
  import("./pages/WorkspaceDetailPage").then((module) => ({
    default: module.WorkspaceDetailPage,
  })),
);

function RouteFallback() {
  return (
    <div className="app-route-loading" role="status" aria-label="正在加载" />
  );
}

const router = createBrowserRouter([
  {
    element: <GuestRoute />,
    children: [
      { path: "/login", element: <LoginPage /> },
      { path: "/register", element: <RegisterPage /> },
    ],
  },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <Layout />,
        children: [
          { path: "/workspaces", element: <WorkspaceListPage /> },
        ],
      },
      {
        path: "/workspaces/:id",
        element: (
          <Suspense fallback={<RouteFallback />}>
            <WorkspaceDetailPage />
          </Suspense>
        ),
      },
    ],
  },
  { path: "*", element: <Navigate to="/workspaces" replace /> },
]);

export default function App() {
  const appearance = useAppearanceStore((state) => state.appearance);

  useEffect(() => {
    document.documentElement.dataset.theme = appearance;
  }, [appearance]);

  return <RouterProvider router={router} />;
}
