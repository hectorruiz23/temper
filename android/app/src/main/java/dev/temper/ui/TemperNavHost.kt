package dev.temper.ui

import android.net.Uri
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import dev.temper.AppContainer
import dev.temper.ui.result.ResultScreen
import dev.temper.ui.runner.RunnerScreen
import dev.temper.ui.session.SessionScreen
import dev.temper.ui.sets.SetsScreen
import dev.temper.ui.settings.SettingsScreen
import dev.temper.ui.week.WeekScreen

private object Routes {
    const val Week = "week"
    const val Settings = "settings"
    const val Session = "session/{date}/{templateId}"
    const val Sets = "sets/{sessionId}/{templateId}/{date}/{revision}"
    const val Result = "result/{sessionId}/{date}/{revision}"
    const val Runner = "runner/{date}/{templateId}/{planVersionId}"

    fun session(date: String, templateId: String): String = "session/${Uri.encode(date)}/${Uri.encode(templateId)}"

    fun sets(sessionId: String, templateId: String, date: String, revision: Long): String =
        "sets/${Uri.encode(sessionId)}/${Uri.encode(templateId)}/${Uri.encode(date)}/$revision"

    fun result(sessionId: String, date: String, revision: Long): String =
        "result/${Uri.encode(sessionId)}/${Uri.encode(date)}/$revision"

    fun runner(date: String, templateId: String, planVersionId: Long): String =
        "runner/${Uri.encode(date)}/${Uri.encode(templateId)}/$planVersionId"
}

@Composable
fun TemperNavHost(container: AppContainer) {
    val navController = rememberNavController()
    val startDestination = remember {
        if (container.configuration.value == null) Routes.Settings else Routes.Week
    }

    NavHost(navController = navController, startDestination = startDestination) {
        composable(Routes.Week) {
            WeekScreen(
                container = container,
                onOpenSession = { date, templateId -> navController.navigate(Routes.session(date, templateId)) },
                onOpenRunner = { date, templateId, planVersionId ->
                    navController.navigate(Routes.runner(date, templateId, planVersionId))
                },
                onOpenSettings = { navController.navigate(Routes.Settings) { launchSingleTop = true } },
            )
        }
        composable(
            route = Routes.Runner,
            arguments = listOf(
                navArgument("date") { type = NavType.StringType },
                navArgument("templateId") { type = NavType.StringType },
                navArgument("planVersionId") { type = NavType.LongType },
            ),
        ) { entry ->
            val date = Uri.decode(entry.arguments?.getString("date").orEmpty())
            val templateId = Uri.decode(entry.arguments?.getString("templateId").orEmpty())
            val planVersionId = entry.arguments?.getLong("planVersionId") ?: 0L
            RunnerScreen(
                container = container,
                date = date,
                templateId = templateId,
                planVersionId = planVersionId,
                onBack = { navController.popBackStack() },
                onOpenSettings = { navController.navigate(Routes.Settings) { launchSingleTop = true } },
                onDone = { navController.popBackStack(Routes.Week, inclusive = false) },
            )
        }
        composable(Routes.Settings) {
            SettingsScreen(
                container = container,
                canGoBack = navController.previousBackStackEntry != null,
                onConfigured = {
                    navController.navigate(Routes.Week) {
                        popUpTo(Routes.Settings) { inclusive = true }
                        launchSingleTop = true
                    }
                },
                onBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.Session,
            arguments = listOf(
                navArgument("date") { type = NavType.StringType },
                navArgument("templateId") { type = NavType.StringType },
            ),
        ) { entry ->
            val date = Uri.decode(entry.arguments?.getString("date").orEmpty())
            val templateId = Uri.decode(entry.arguments?.getString("templateId").orEmpty())
            SessionScreen(
                container = container,
                date = date,
                templateId = templateId,
                onBack = { navController.popBackStack() },
                onOpenSettings = { navController.navigate(Routes.Settings) { launchSingleTop = true } },
                onStarted = { sessionId, revision ->
                    navController.navigate(Routes.sets(sessionId, templateId, date, revision))
                },
            )
        }
        composable(
            route = Routes.Sets,
            arguments = listOf(
                navArgument("sessionId") { type = NavType.StringType },
                navArgument("templateId") { type = NavType.StringType },
                navArgument("date") { type = NavType.StringType },
                navArgument("revision") { type = NavType.LongType },
            ),
        ) { entry ->
            val sessionId = Uri.decode(entry.arguments?.getString("sessionId").orEmpty())
            val templateId = Uri.decode(entry.arguments?.getString("templateId").orEmpty())
            val date = Uri.decode(entry.arguments?.getString("date").orEmpty())
            val revision = entry.arguments?.getLong("revision") ?: 1L
            SetsScreen(
                container = container,
                sessionId = sessionId,
                templateId = templateId,
                date = date,
                revision = revision,
                onBack = { navController.popBackStack() },
                onOpenSettings = { navController.navigate(Routes.Settings) { launchSingleTop = true } },
                onContinue = { navController.navigate(Routes.result(sessionId, date, revision)) },
            )
        }
        composable(
            route = Routes.Result,
            arguments = listOf(
                navArgument("sessionId") { type = NavType.StringType },
                navArgument("date") { type = NavType.StringType },
                navArgument("revision") { type = NavType.LongType },
            ),
        ) { entry ->
            val sessionId = Uri.decode(entry.arguments?.getString("sessionId").orEmpty())
            val date = Uri.decode(entry.arguments?.getString("date").orEmpty())
            val revision = entry.arguments?.getLong("revision") ?: 1L
            ResultScreen(
                container = container,
                sessionId = sessionId,
                date = date,
                revision = revision,
                onBack = { navController.popBackStack() },
                onOpenSettings = { navController.navigate(Routes.Settings) { launchSingleTop = true } },
                onDone = { navController.popBackStack(Routes.Week, inclusive = false) },
            )
        }
    }
}
