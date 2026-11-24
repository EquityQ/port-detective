"""
port_detective.errors
~~~~~~~~~~~~~~~~~~~~~

Exception hierarchy for the port-detective Python SDK.
"""


class PortDetectiveError(Exception):
    """Base exception for port-detective errors."""


class PortDetectiveNotFoundError(PortDetectiveError):
    """Raised when the port-detective executable cannot be found."""


class PortDetectiveVersionError(PortDetectiveError):
    """Raised when an external CLI does not match the SDK's major/minor API."""


class PortDetectiveExecutionError(PortDetectiveError):
    """Raised when port-detective exits with a non-zero exit code."""

    def __init__(self, message: str, returncode: int, stderr: str = "", stdout: str = ""):
        super().__init__(message)
        self.returncode = returncode
        self.stderr = stderr
        self.stdout = stdout
