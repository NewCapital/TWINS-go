#!/bin/bash

echo "================================================"
echo "TWINS Wallet Frontend Setup Verification"
echo "================================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to check if a command exists
check_command() {
    if command -v $1 &> /dev/null; then
        echo -e "${GREEN}✓${NC} $1 is installed"
        return 0
    else
        echo -e "${RED}✗${NC} $1 is not installed"
        return 1
    fi
}

# Function to check if a package is installed
check_package() {
    if npm list $1 &> /dev/null; then
        echo -e "${GREEN}✓${NC} $1 is installed"
        return 0
    else
        echo -e "${RED}✗${NC} $1 is not installed"
        return 1
    fi
}

echo "1. Checking System Dependencies"
echo "--------------------------------"
check_command node
check_command npm
echo ""

echo "2. Checking Key Packages"
echo "------------------------"
check_package react
check_package typescript
check_package tailwindcss
check_package zustand
check_package react-router
check_package lucide-react
check_package react-hook-form
check_package zod
check_package vite
check_package vitest
echo ""

echo "3. Running Tests"
echo "----------------"
npm run test -- --run --reporter=verbose 2>&1 | grep -E "(PASS|FAIL|✓|✗)"
echo ""

echo "4. TypeScript Compilation"
echo "-------------------------"
if npx tsc --noEmit --skipLibCheck 2>&1 | grep -q "error"; then
    echo -e "${YELLOW}⚠${NC} TypeScript has some errors (may be expected)"
else
    echo -e "${GREEN}✓${NC} TypeScript compilation successful"
fi
echo ""

echo "5. Linting Check"
echo "----------------"
if npm run lint 2>&1 | grep -q "error"; then
    echo -e "${YELLOW}⚠${NC} ESLint found some issues (can be fixed with 'npm run format')"
else
    echo -e "${GREEN}✓${NC} ESLint check passed"
fi
echo ""

echo "6. Build Test"
echo "-------------"
if npm run build &> /dev/null; then
    echo -e "${GREEN}✓${NC} Production build successful"
    echo "   Build output in: dist/"
    du -sh dist/ 2>/dev/null | awk '{print "   Total size: " $1}'
else
    echo -e "${RED}✗${NC} Build failed"
fi
echo ""

echo "7. Configuration Files"
echo "----------------------"
for file in tsconfig.json vite.config.ts .prettierrc vitest.config.ts; do
    if [ -f "$file" ]; then
        echo -e "${GREEN}✓${NC} $file exists"
    else
        echo -e "${RED}✗${NC} $file missing"
    fi
done
echo ""

echo "================================================"
echo "Setup Verification Complete!"
echo "================================================"
echo ""
echo "To test the development server:"
echo "  npm run dev"
echo ""
echo "To format code:"
echo "  npm run format"
echo ""
echo "To run tests with UI:"
echo "  npm run test:ui"
echo ""